package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// From ROCKNIX on the RG DS: EmulationStation's A is button 1, while the
// SDL mapping DB puts "a" on button 0.
const esInputCfg = `<?xml version="1.0"?>
<inputList>
	<inputConfig type="keyboard" deviceName="Keyboard" deviceGUID="-1">
		<input name="a" type="key" id="13" value="1" />
		<input name="b" type="key" id="27" value="1" />
	</inputConfig>
	<inputConfig type="joystick" deviceName="retrogame_joypad" deviceGUID="19009b4d4b4800000111000000010000">
		<input name="a" type="button" id="1" value="1" />
		<input name="b" type="button" id="0" value="1" />
		<input name="x" type="button" id="2" value="1" />
	</inputConfig>
	<inputConfig type="joystick" deviceName="retrogame_joypad_s1_f2" deviceGUID="1900adda4b4800001211000000010000">
		<input name="a" type="button" id="1" value="1" />
		<input name="b" type="button" id="0" value="1" />
	</inputConfig>
</inputList>
`

func TestFrontendSwapsAB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "es_input.cfg")
	if err := os.WriteFile(path, []byte(esInputCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	fb := loadFrontendButtons(path)
	for _, c := range []struct {
		guid, name, mapping string
		want                bool
	}{
		{"19009b4d4b4800000111000000010000", "retrogame_joypad", "19009b4d4b4800000111000000010000,retrogame_joypad,platform:Linux,x:b2,a:b0,b:b1,y:b3,", true},
		{"1900ADDA4B4800001211000000010000", "retrogame_joypad_s1_f2", "1900adda4b4800001211000000010000,retrogame_joypad_s1_f2,platform:Linux,x:b2,a:b1,b:b0,y:b3,", false},
		{"03009b4d4b4800000111000000010000", "retrogame_joypad", "03009b4d4b4800000111000000010000,retrogame_joypad,a:b0,b:b1,", true}, // found by name
		{"0300aaaa000000000000000000000000", "Other Pad", "0300aaaa000000000000000000000000,Other Pad,a:b0,b:b1,", false},
		{"19009b4d4b4800000111000000010000", "retrogame_joypad", "19009b4d4b4800000111000000010000,retrogame_joypad,a:b1,b:b2,", false},
	} {
		if got := fb.swapsAB(c.guid, c.name, c.mapping); got != c.want {
			t.Errorf("swapsAB(%s, %q) = %v, want %v", c.guid, c.mapping, got, c.want)
		}
	}
	if fb := loadFrontendButtons(filepath.Join(t.TempDir(), "missing.cfg")); fb.swapsAB("19009b4d4b4800000111000000010000", "retrogame_joypad", "a:b0,b:b1") {
		t.Error("swapsAB without es_input.cfg = true")
	}
}
