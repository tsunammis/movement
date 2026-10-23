package contract

import (
	"encoding/json"
	"reflect"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"movement.local/movement/internal/domain"
)

var validators sync.Map

func Validate(t reflect.Type, raw []byte) error {
	cached, ok := validators.Load(t)
	if !ok {
		b, err := json.Marshal(Schema(t, true))
		if err != nil {
			return err
		}
		var schema jsonschema.Schema
		if err = json.Unmarshal(b, &schema); err != nil {
			return err
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			return err
		}
		cached, _ = validators.LoadOrStore(t, resolved)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return cached.(*jsonschema.Resolved).Validate(value)
}
func ValidateTask(raw []byte) error  { return Validate(reflect.TypeFor[domain.Task](), raw) }
func ValidateEvent(raw []byte) error { return Validate(reflect.TypeFor[domain.Event](), raw) }
