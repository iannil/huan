#!/usr/bin/env python3
"""Patch a Go-built darwin/arm64 plugin .so so macOS 27 dyld accepts it.

Go's linker appends the __DWARF segment (machoCombineDwarf) after writing the
LC_DYLD_CHAINED_FIXUPS payload, leaving seg_count one short of the real
LC_SEGMENT_64 count. dyld in macOS 27 validates equality and rejects the file
("chained fixups, seg_count does not match number of segments").

Fix: rewrite the dyld_chained_starts_in_image in place with seg_count equal to
the real segment count, compacting the per-segment structs and leaving
segments without fixup chains as empty entries (offset 0).
"""
import struct, sys

def main(path):
    with open(path, 'rb') as f:
        data = bytearray(f.read())

    ncmds = struct.unpack_from('<I', data, 16)[0]
    off, fixoff = 32, None
    segs = 0
    for _ in range(ncmds):
        cmd, cmdsize = struct.unpack_from('<2I', data, off)
        if cmd == 0x19:
            segs += 1
        if cmd == 0x80000034:
            dataoff, datasize = struct.unpack_from('<2I', data, off + 8)
            fixoff = off
        off += cmdsize
    if fixoff is None:
        print(f"{path}: no LC_DYLD_CHAINED_FIXUPS, nothing to do")
        return

    hdr = struct.unpack_from('<4I', data, dataoff)  # version, starts, imports, symbols
    starts = dataoff + hdr[1]
    seg_count = struct.unpack_from('<I', data, starts)[0]
    if seg_count == segs:
        print(f"{path}: seg_count already {segs}, nothing to do")
        return
    if seg_count > segs:
        sys.exit(f"{path}: seg_count {seg_count} > segments {segs}; unexpected layout")

    offs = list(struct.unpack_from('<%dI' % seg_count, data, starts + 4))
    starts_len = hdr[2] - hdr[1]                       # current starts section size
    new_hdr_len = 4 + 4 * segs                         # seg_count + offsets array
    # Compact: walk old structs in order, re-emit after the new header.
    blobs, new_offs = [], []
    cur = new_hdr_len
    for so in offs:
        if so == 0:
            new_offs.append(0)
            continue
        size = struct.unpack_from('<I', data, starts + so)[0]
        blobs.append(bytes(data[starts + so: starts + so + size]))
        new_offs.append(cur)
        cur += size
    new_offs += [0] * (segs - len(new_offs))   # appended segments carry no fixups
    total = new_hdr_len + sum(len(b) for b in blobs)
    if total > starts_len:
        sys.exit(f"{path}: needs {total} bytes, only {starts_len} available")

    out = struct.pack('<I', segs) + struct.pack('<%dI' % segs, *new_offs) + b''.join(blobs)
    out += b'\0' * (starts_len - total)                # keep section size stable
    data[starts:starts + starts_len] = out

    with open(path, 'wb') as f:
        f.write(data)
    print(f"{path}: patched seg_count {seg_count} -> {segs}")

if __name__ == '__main__':
    for p in sys.argv[1:]:
        main(p)
