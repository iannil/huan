package plugin

// Schema describes the full config shape a plugin expects.
type Schema struct {
	Fields []FieldSchema
}

// FieldSchema describes a single config field.
type FieldSchema struct {
	Key         string // 字段名，对应 yaml key
	Type        string // "string" | "int" | "bool" | "string_slice" | "map"
	Required    bool   // true = 必填，启动时校验
	Default     any    // 默认值（Required=false 时生效）
	Description string // 人类可读的说明
	Sensitive   bool   // true = 在 CLI info 中 mask 为 ***
	EnvVarHint  string // 建议的环境变量名，仅用于文档提示
}

// SchemaProvider is an optional interface plugins can implement to declare
// their config schema. Used by the registry for config validation.
type SchemaProvider interface {
	ConfigSchema() Schema
}
