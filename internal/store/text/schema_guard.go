//go:build darwin

package text

import (
	"fmt"
	"os"
	"strings"

	"movement.local/movement/internal/domain"
)

// Guard the whole directory before recovery, staging cleanup, or mutation.
// A transaction may have installed only its event or task; this check reads
// versions without demanding their final revision/receipt consistency.
func (d *disk) checkStoreSchemas() error {
	raw, err := d.read("config.json", MaxRecordBytes)
	if err == nil {
		if err = checkJSONSchema(raw, "config", MaxRecordBytes); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	tasks, err := d.entries("tasks")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if !task.IsDir() || !domain.ValidID(task.Name()) {
			return fmt.Errorf("invalid task directory entry")
		}
		path := "tasks/" + task.Name()
		raw, err = d.read(path+"/task.json", MaxRecordBytes)
		if err == nil {
			if err = checkJSONSchema(raw, "task", MaxRecordBytes); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		history, err := d.entries(path + "/history")
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, event := range history {
			// An interrupted immutable installation can leave a temporary event file.
			if strings.HasSuffix(event.Name(), ".tmp") {
				continue
			}
			if !strings.HasSuffix(event.Name(), ".md") {
				return fmt.Errorf("unexpected history entry %s", event.Name())
			}
			raw, err = d.read(path+"/history/"+event.Name(), MaxRecordBytes)
			if err != nil {
				return err
			}
			if _, err = parseEvent(raw); err != nil {
				return err
			}
		}
	}
	return nil
}
