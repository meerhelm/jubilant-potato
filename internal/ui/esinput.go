package ui

import (
	"encoding/xml"
	"os"
	"strconv"
	"strings"
)

// frontendAB is the raw joystick buttons the firmware's frontend treats as
// A and B for one pad.
type frontendAB struct{ a, b int }

// frontendButtons is what EmulationStation's es_input.cfg says about A and B,
// by joystick GUID and by device name.
type frontendButtons struct {
	byGUID, byName map[string]frontendAB
}

// loadFrontendButtons reads an es_input.cfg; a missing or broken file gives
// an empty result.
func loadFrontendButtons(path string) frontendButtons {
	fb := frontendButtons{byGUID: map[string]frontendAB{}, byName: map[string]frontendAB{}}
	if path == "" {
		return fb
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fb
	}
	var list struct {
		Configs []struct {
			Type   string `xml:"type,attr"`
			Name   string `xml:"deviceName,attr"`
			GUID   string `xml:"deviceGUID,attr"`
			Inputs []struct {
				Name string `xml:"name,attr"`
				Type string `xml:"type,attr"`
				ID   string `xml:"id,attr"`
			} `xml:"input"`
		} `xml:"inputConfig"`
	}
	if xml.Unmarshal(b, &list) != nil {
		return fb
	}
	for _, c := range list.Configs {
		if c.Type != "joystick" {
			continue
		}
		ab := frontendAB{-1, -1}
		for _, in := range c.Inputs {
			id, err := strconv.Atoi(in.ID)
			if in.Type != "button" || err != nil {
				continue
			}
			switch in.Name {
			case "a":
				ab.a = id
			case "b":
				ab.b = id
			}
		}
		if ab.a < 0 || ab.b < 0 || ab.a == ab.b {
			continue
		}
		if c.GUID != "" {
			fb.byGUID[strings.ToLower(c.GUID)] = ab
		}
		if _, dup := fb.byName[c.Name]; c.Name != "" && !dup {
			fb.byName[c.Name] = ab
		}
	}
	return fb
}

// swapsAB reports whether an SDL mapping puts A and B on the opposite buttons
// from the frontend. ROCKNIX's mapping DB is positional on some handhelds
// (SDL "a" is the bottom face button, printed B) while EmulationStation
// follows the printed labels.
func (fb frontendButtons) swapsAB(guid, name, mapping string) bool {
	ab, ok := fb.byGUID[strings.ToLower(guid)]
	if !ok {
		if ab, ok = fb.byName[name]; !ok {
			return false
		}
	}
	sdlA, sdlB := mappingButton(mapping, "a"), mappingButton(mapping, "b")
	return sdlA == ab.b && sdlB == ab.a
}

// mappingButton returns the raw button behind key in an SDL mapping string
// ("a:b0" -> 0), or -1.
func mappingButton(mapping, key string) int {
	for _, f := range strings.Split(mapping, ",") {
		k, v, ok := strings.Cut(f, ":")
		if !ok || k != key {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(v, "b")); err == nil && strings.HasPrefix(v, "b") {
			return n
		}
		return -1
	}
	return -1
}
