package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// AdminConfig holds application-wide admin settings (singleton row).
type AdminConfig struct {
	ent.Schema
}

func (AdminConfig) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").Default(1), // singleton: always ID 1
		field.String("otel_endpoint").Optional(),
		field.Bool("traces_enabled").Default(false),
		field.Bool("metrics_enabled").Default(false),
		field.Bool("logs_enabled").Default(false),
		field.String("log_severity").Default("warning"),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
