//go:build darwin

package text

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// Verification summarizes a recovered and fully validated authoritative store.
// It is not a snapshot handle: callers must stop other processes before copying.
type Verification struct {
	Tasks    int `json:"tasks"`
	Events   int `json:"events"`
	Receipts int `json:"receipts"`
}

func (d *disk) verify() (out Verification, err error) {
	bundles, err := d.snapshot()
	if err != nil {
		return out, err
	}
	out.Tasks = len(bundles)
	for _, bundle := range bundles {
		out.Events += len(bundle.History)
	}
	entries, err := d.entries("receipts")
	if err != nil {
		return out, err
	}
	out.Receipts = len(entries)
	return out, nil
}

func (r *Repository) Verify(ctx context.Context) (out Verification, err error) {
	if _, err = r.Settings(ctx); err != nil {
		return out, err
	}
	err = r.locked(ctx, func(d *disk) error { var e error; out, e = d.verify(); return e })
	return
}

// RotateAPIToken replaces only the HTTP credential. Stop all HTTP servers first:
// existing processes hold their credential and sessions in memory until exit.
// Claim tokens in authoritative receipts remain valid and are not rewritten.
func (r *Repository) RotateAPIToken(ctx context.Context) error {
	if _, err := r.Settings(ctx); err != nil {
		return err
	}
	return r.locked(ctx, func(d *disk) error {
		if _, err := d.verify(); err != nil {
			return err
		}
		if err := d.safe("runtime/api-token", true); err != nil {
			return err
		}
		const temporary = "runtime/api-token.tmp"
		if err := d.remove(temporary); err != nil {
			return err
		}
		var secret [32]byte
		if _, err := rand.Read(secret[:]); err != nil {
			return err
		}
		if err := d.write(temporary, []byte(hex.EncodeToString(secret[:])+"\n")); err != nil {
			return err
		}
		if err := d.rename(temporary, "runtime/api-token"); err != nil {
			return err
		}
		return d.syncDir("runtime")
	})
}
