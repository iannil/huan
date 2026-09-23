package build

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const markdownDiskMagic = "HUANMD01"
const maxMarkdownDiskEntry = 64 << 20

type markdownDisk struct {
	dir, namespace string
	maxBytes       int64
	mu             sync.Mutex
	lease          *os.File
	used, reserved int64
	records        map[string]*list.Element
	oldest         list.List
	inflight       map[string]bool
}

type markdownDiskRecord struct {
	name string
	cost int64
}

// Charge a minimum per file as well as its bytes so many tiny records cannot
// grow the in-memory directory index independently of the capacity budget.
func markdownDiskCost(size int64) int64 { return max(size, 512) }

// NewPersistentMarkdownCache adds an optional best-effort disk tier. namespace
// must identify the renderer implementation (the CLI hashes its executable).
// Invalid, missing or unreadable records are misses, never build failures.
// Only the instance holding the directory's advisory lease writes. Other
// instances remain read-only. Call Close after all builds have stopped.
func NewPersistentMarkdownCache(memoryBytes int64, dir, namespace string, diskBytes int64) (*MarkdownCache, error) {
	c := NewMarkdownCache(memoryBytes)
	if diskBytes <= 0 {
		return c, nil
	}
	if dir == "" || namespace == "" {
		return nil, fmt.Errorf("markdown cache requires a directory and renderer namespace")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	d := &markdownDisk{dir: dir, namespace: namespace, maxBytes: diskBytes,
		records: make(map[string]*list.Element), inflight: make(map[string]bool)}
	// Failure to acquire the optional writer lease still permits disk reads.
	d.lease, _ = acquireMarkdownLease(dir)
	if d.lease != nil {
		if err := d.initialize(); err != nil {
			_ = d.lease.Close()
			return nil, err
		}
	}
	c.disk = d
	return c, nil
}

// Close releases the optional disk writer lease. It is idempotent and nil-safe.
// The caller must stop all builds before closing; subsequent puts use memory
// only, while completed disk records remain readable by this or other caches.
func (c *MarkdownCache) Close() error {
	if c == nil || c.disk == nil {
		return nil
	}
	d := c.disk
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lease == nil {
		return nil
	}
	err := d.lease.Close()
	d.lease = nil
	return err
}

func (d *markdownDisk) initialize() error {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return err
	}
	type existing struct {
		name string
		info os.FileInfo
	}
	var records []existing
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".markdown-") {
			// The exclusive lifetime lease proves no cooperating writer can
			// still be using these files after an earlier process exited.
			if err := os.Remove(filepath.Join(d.dir, name)); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if !strings.HasSuffix(name, ".mdcache") {
			continue
		}
		stem := strings.TrimSuffix(name, ".mdcache")
		if len(stem) != 64 {
			continue
		}
		if _, err := hex.DecodeString(stem); err != nil {
			continue
		}
		info, err := entry.Info()
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		records = append(records, existing{name, info})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].info.ModTime().Before(records[j].info.ModTime()) })
	var total int64
	for _, record := range records {
		total += markdownDiskCost(record.info.Size())
	}
	// Recover over-budget directories before allocating the retained map/list.
	// The scan metadata is temporary; the live index obeys the capacity budget.
	for total > d.maxBytes {
		record := records[0]
		if err := os.Remove(filepath.Join(d.dir, record.name)); err != nil && !os.IsNotExist(err) {
			return err
		}
		total -= markdownDiskCost(record.info.Size())
		records = records[1:]
	}
	for _, record := range records {
		d.addRecord(record.name, markdownDiskCost(record.info.Size()))
	}
	return nil
}

// All accounting helpers run with mu held (or during construction).
func (d *markdownDisk) addRecord(name string, cost int64) {
	d.records[name] = d.oldest.PushBack(markdownDiskRecord{name, cost})
	d.used += cost
}

func (d *markdownDisk) removeRecord(e *list.Element) error {
	record := e.Value.(markdownDiskRecord)
	if err := os.Remove(filepath.Join(d.dir, record.name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	d.used -= record.cost
	delete(d.records, record.name)
	d.oldest.Remove(e)
	return nil
}

func (d *markdownDisk) makeRoom(cost int64) error {
	for d.used+d.reserved > d.maxBytes-cost {
		e := d.oldest.Front()
		if e == nil {
			return fmt.Errorf("markdown disk cache capacity reserved by active writes")
		}
		if err := d.removeRecord(e); err != nil {
			return err
		}
	}
	return nil
}

func (d *markdownDisk) reserve(name string, cost int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lease == nil || d.inflight[name] || cost > d.maxBytes {
		return false
	}
	// Remove a prior/corrupt copy first; the new temporary file needs its own
	// capacity and repairing a full cache must not require double that space.
	if e := d.records[name]; e != nil {
		if err := d.removeRecord(e); err != nil {
			return false
		}
	}
	if err := d.makeRoom(cost); err != nil {
		return false
	}
	d.inflight[name] = true
	d.reserved += cost
	return true
}

func (d *markdownDisk) finish(name, retained string, cost int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.inflight, name)
	d.reserved -= cost
	if retained != "" {
		d.addRecord(retained, cost)
	}
}

func (d *markdownDisk) id(key markdownKey) [32]byte {
	h := sha256.New()
	_, _ = io.WriteString(h, d.namespace)
	_, _ = h.Write(key.Raw[:])
	_, _ = h.Write(key.Expanded[:])
	_, _ = h.Write(key.Markup[:])
	var id [32]byte
	copy(id[:], h.Sum(nil))
	return id
}

func (d *markdownDisk) path(id [32]byte) string {
	return filepath.Join(d.dir, hex.EncodeToString(id[:])+".mdcache")
}
func (d *markdownDisk) limit() int64 {
	if d.maxBytes < maxMarkdownDiskEntry {
		return d.maxBytes
	}
	return maxMarkdownDiskEntry
}

func (d *markdownDisk) load(key markdownKey) (markdownValue, bool) {
	id := d.id(key)
	f, err := os.Open(d.path(id))
	if err != nil {
		return markdownValue{}, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 72 || info.Size() > d.limit() {
		return markdownValue{}, false
	}
	data, ok := readSizedMarkdownRecord(f, info.Size())
	if !ok || string(data[:8]) != markdownDiskMagic || !bytes.Equal(data[8:40], id[:]) {
		return markdownValue{}, false
	}
	sum := sha256.Sum256(data[72:])
	if !bytes.Equal(data[40:72], sum[:]) {
		return markdownValue{}, false
	}
	payload := data[72:]
	if len(payload) < 8 {
		return markdownValue{}, false
	}
	words := binary.LittleEndian.Uint64(payload[:8])
	payload = payload[8:]
	if words > uint64(^uint(0)>>1) {
		return markdownValue{}, false
	}
	value := markdownValue{WordCount: int(words)}
	for _, field := range []*string{&value.HTML, &value.Plain, &value.Summary} {
		if len(payload) < 8 {
			return markdownValue{}, false
		}
		n := binary.LittleEndian.Uint64(payload[:8])
		payload = payload[8:]
		if n > uint64(len(payload)) {
			return markdownValue{}, false
		}
		*field = string(payload[:int(n)])
		payload = payload[int(n):]
	}
	return value, len(payload) == 0
}

// readSizedMarkdownRecord allocates once for a stat-sized record and probes one
// extra byte. Atomic publication normally keeps the size stable; truncation,
// growth or read failures are cache misses rather than partial records.
func readSizedMarkdownRecord(r io.Reader, size int64) ([]byte, bool) {
	if size < 0 || size > maxMarkdownDiskEntry {
		return nil, false
	}
	data := make([]byte, int(size)+1)
	n, err := io.ReadFull(r, data)
	if int64(n) != size || err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, false
	}
	return data[:n], true
}

func (d *markdownDisk) store(key markdownKey, value markdownValue) {
	size := int64(104) + int64(len(value.HTML)) + int64(len(value.Plain)) + int64(len(value.Summary))
	if size > d.limit() || value.WordCount < 0 {
		return
	}
	id := d.id(key)
	name := hex.EncodeToString(id[:]) + ".mdcache"
	cost := markdownDiskCost(size)
	if !d.reserve(name, cost) {
		return
	}
	retained := ""
	defer func() { d.finish(name, retained, cost) }()
	record := make([]byte, 0, int(size))
	record = append(record, markdownDiskMagic...)
	record = append(record, id[:]...)
	record = append(record, make([]byte, sha256.Size)...)
	record = binary.LittleEndian.AppendUint64(record, uint64(value.WordCount))
	for _, s := range []string{value.HTML, value.Plain, value.Summary} {
		record = binary.LittleEndian.AppendUint64(record, uint64(len(s)))
		record = append(record, s...)
	}
	sum := sha256.Sum256(record[72:])
	copy(record[40:72], sum[:])
	f, err := os.CreateTemp(d.dir, ".markdown-*")
	if err != nil {
		return
	}
	tmp := f.Name()
	defer func() {
		if retained == name {
			// Successful rename consumed the temporary path; avoid an extra
			// failing unlink for every persisted record.
			return
		}
		if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
			// Keep a failed cleanup charged and eligible for later eviction.
			retained = filepath.Base(tmp)
		}
	}()
	if _, err = f.Write(record); err != nil {
		_ = f.Close()
		return
	}
	if err = f.Close(); err != nil {
		return
	}
	// Publication is atomic. Readers can miss an evicted/replaced record but
	// never observe a partially written one.
	if err := os.Rename(tmp, d.path(id)); err == nil {
		retained = name
	}
}

// PruneDisk maintains the writer's indexed budget without rescanning disk.
// Writes already reserve capacity before creating files. Read-only instances
// never prune another process's cache; unrelated files are never touched.
func (c *MarkdownCache) PruneDisk() error {
	if c == nil || c.disk == nil {
		return nil
	}
	d := c.disk
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lease == nil {
		return nil
	}
	return d.makeRoom(0)
}
