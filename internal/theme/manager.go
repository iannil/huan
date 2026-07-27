package theme

import (
	"fmt"
	"sync"

	"github.com/iannil/huan/internal/plugin"
)

// Manager 管理主题插件的生命周期和状态。
type Manager struct {
	mu         sync.RWMutex
	registry   *plugin.Registry
	active     ThemePlugin
	activeName string
}

// NewManager 创建主题管理器。
func NewManager(registry *plugin.Registry) *Manager {
	return &Manager{registry: registry}
}

// Activate 激活指定名称的主题插件。
func (m *Manager) Activate(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.registry.Get(name)
	if !ok {
		return fmt.Errorf("theme: plugin %q not found", name)
	}
	tp, ok := p.(ThemePlugin)
	if !ok {
		return fmt.Errorf("theme: plugin %q does not implement ThemePlugin", name)
	}

	m.active = tp
	m.activeName = name
	return nil
}

// Deactivate 停用当前激活的主题。
func (m *Manager) Deactivate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = nil
	m.activeName = ""
}

// Active 返回当前激活的主题。
func (m *Manager) Active() ThemePlugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// ActiveName 返回当前激活主题的名称。
func (m *Manager) ActiveName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeName
}

// ListAvailable 列出所有注册了 ThemePlugin 能力的插件。
func (m *Manager) ListAvailable() []ThemePlugin {
	return plugin.Find[ThemePlugin](m.registry)
}