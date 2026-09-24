package i18n

import "testing"

func TestLanguagesHaveRequiredMessages(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"en", "zh_cn", "zh_cn_moe"} {
		if _, err := Load(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
