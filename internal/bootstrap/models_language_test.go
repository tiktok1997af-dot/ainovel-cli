package bootstrap

import "testing"

func TestModelSetNormalizedLanguage(t *testing.T) {
	var nilSet *ModelSet
	if got := nilSet.NormalizedLanguage(); got != "vi" {
		t.Fatalf("nil ModelSet should safely default to vi, got %q", got)
	}

	cases := []struct {
		name string
		lang string
		want string
	}{
		{"vietnamese", "vi", "vi"},
		{"empty defaults vietnamese", "", "vi"},
		{"english follows config normalization", "en", "vi"},
		{"chinese zh", "zh", "zh"},
		{"chinese alias", "chinese", "zh"},
		{"chinese cn", "cn", "zh"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ms := &ModelSet{config: Config{Language: tc.lang}}
			if got := ms.NormalizedLanguage(); got != tc.want {
				t.Fatalf("language %q: want %q, got %q", tc.lang, tc.want, got)
			}
		})
	}
}
