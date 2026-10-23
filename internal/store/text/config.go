//go:build darwin

package text

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"movement.local/movement/internal/app"
	"movement.local/movement/internal/domain"
)

type Config struct {
	SchemaVersion int    `json:"schema_version"`
	Timezone      string `json:"timezone"`
}

func (r *Repository) Initialize(ctx context.Context, zone string) (out Config, err error) {
	if zone == "" {
		zone = "UTC"
	}
	if _, e := time.LoadLocation(zone); e != nil {
		return out, domain.Err("invalid_input", "unknown IANA timezone")
	}
	err = r.locked(ctx, func(d *disk) error {
		raw, e := d.read("config.json", MaxRecordBytes)
		if e == nil {
			if e = app.Decode(raw, &out); e != nil {
				return e
			}
			if out.SchemaVersion != 1 {
				return domain.Err("unsupported_schema", "unsupported config schema")
			}
		} else if os.IsNotExist(e) {
			out = Config{1, zone}
			if e = d.immutable("config.json", encode(out)); e != nil {
				return e
			}
		} else {
			return e
		}
		raw, e = d.read("runtime/api-token", 256)
		if e == nil {
			if len(strings.TrimSpace(string(raw))) != 64 {
				return fmt.Errorf("invalid API token file")
			}
			return nil
		}
		if !os.IsNotExist(e) {
			return e
		}
		var secret [32]byte
		if _, e = rand.Read(secret[:]); e != nil {
			return e
		}
		return d.immutable("runtime/api-token", []byte(hex.EncodeToString(secret[:])+"\n"))
	})
	return
}
func (r *Repository) Settings(ctx context.Context) (out Config, err error) {
	err = r.locked(ctx, func(d *disk) error {
		raw, e := d.read("config.json", MaxRecordBytes)
		if os.IsNotExist(e) {
			return domain.Err("not_initialized", "run movement init first")
		}
		if e != nil {
			return e
		}
		if e = app.Decode(raw, &out); e != nil {
			return e
		}
		if out.SchemaVersion != 1 {
			return domain.Err("unsupported_schema", "unsupported config schema")
		}
		if _, e = time.LoadLocation(out.Timezone); e != nil {
			return fmt.Errorf("invalid stored timezone")
		}
		return nil
	})
	return
}
func (r *Repository) APIToken(ctx context.Context) (out string, err error) {
	err = r.locked(ctx, func(d *disk) error {
		raw, e := d.read("runtime/api-token", 256)
		if e != nil {
			return e
		}
		out = strings.TrimSpace(string(raw))
		if len(out) != 64 {
			return fmt.Errorf("invalid API token")
		}
		return nil
	})
	return
}
