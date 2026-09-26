package romname

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		file string
		want Info
	}{
		{"Legend of Zelda, The - A Link to the Past (USA).sfc",
			Info{Title: "Legend of Zelda, The - A Link to the Past", Regions: []string{"USA"}, Languages: []string{"en"}}},
		{"Pokemon - Emerald Version (USA, Europe) (Rev 1).zip",
			Info{Title: "Pokemon - Emerald Version", Regions: []string{"USA", "Europe"}, Languages: []string{"en"}, Revision: 100}},
		{"Super Mario World (Europe) (En,Fr,De).zip",
			Info{Title: "Super Mario World", Regions: []string{"Europe"}, Languages: []string{"en", "fr", "de"}}},
		{"Mother 3 (Japan).gba",
			Info{Title: "Mother 3", Regions: []string{"Japan"}, Languages: []string{"ja"}}},
		{"Chrono Trigger (U) [T+Rus_Chief-Net].smc",
			Info{Title: "Chrono Trigger", Regions: []string{"USA"}, Languages: []string{"ru"}, Kind: Translation}},
		{"Sonic the Hedgehog (JUE) [!].gen",
			Info{Title: "Sonic the Hedgehog", Regions: []string{"Japan", "USA", "Europe"}, Languages: []string{"ja", "en"}, Verified: true}},
		{"Star Fox 2 (Japan) (Beta) (1995-06-20).sfc",
			Info{Title: "Star Fox 2", Regions: []string{"Japan"}, Languages: []string{"ja"}, Kind: Prerelease}},
		{"Contra (U) [b2].nes",
			Info{Title: "Contra", Regions: []string{"USA"}, Languages: []string{"en"}, Kind: Bad}},
		{"Super Mario Bros (W) [h1].nes",
			Info{Title: "Super Mario Bros", Regions: []string{"World"}, Languages: []string{"en"}, Kind: Hack}},
		{"Street Fighter II' (Japan) (Rev A).md",
			Info{Title: "Street Fighter II'", Regions: []string{"Japan"}, Languages: []string{"ja"}, Revision: 1}},
		{"Castlevania (Europe) (v1.1) (Virtual Console).nes",
			Info{Title: "Castlevania", Regions: []string{"Europe"}, Languages: []string{"en"}, Revision: 101}},
		{"Anguna.gba", Info{Title: "Anguna"}},
	}
	for _, c := range cases {
		if got := Parse(c.file); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q)\n got %+v\nwant %+v", c.file, got, c.want)
		}
	}
}
