package buildlog

import "testing"

func TestValidateName(t *testing.T) {
	good := []string{"123z", "0xff", "wow seperator-things", "un_camera"}
	bad := []string{"#", "á", "¡Hola!"}

	for _, name := range good {
		if err := ValidateName(name); err != nil {
			t.Errorf("unexpected error: %v", err.Error())
		}
	}
	for _, name := range bad {
		if err := ValidateName(name); err == nil {
			t.Errorf("failed to reject string: %q", name)
		}
	}
}
