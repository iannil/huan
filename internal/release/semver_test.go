package release

import "testing"

func TestValidateVersion_Valid(t *testing.T) {
	cases := []string{
		"0.1.0",
		"1.0.0",
		"10.20.30",
		"0.1.0-rc1",
		"0.1.0-alpha",
		"0.1.0-alpha.1",
		"0.1.0-alpha.beta",
		"1.0.0+build.5",
		"0.1.0-alpha.1+exp.sha.5114f85",
	}
	for _, c := range cases {
		if err := ValidateVersion(c); err != nil {
			t.Errorf("ValidateVersion(%q) = %v, want nil", c, err)
		}
	}
}

func TestValidateVersion_Invalid(t *testing.T) {
	cases := []struct {
		v    string
		want string
	}{
		{"", "empty"},
		{"v0.1.0", "leading v"},
		{"V0.1.0", "leading v"},
		{"0.1.0\n", "whitespace"},
		{" 0.1.0", "whitespace"},
		{"0.1", "not semver"},
		{"0.1.0.0", "not semver"},
		{"latest", "not semver"},
		{"dev", "not semver"},
		{"0.1.0-", "not semver"},
		{"01.0.0", "not semver"}, // leading zero
	}
	for _, c := range cases {
		err := ValidateVersion(c.v)
		if err == nil {
			t.Errorf("ValidateVersion(%q) = nil, want error", c.v)
			continue
		}
		if !contains(err.Error(), c.want) {
			t.Errorf("ValidateVersion(%q) error %q, want substring %q", c.v, err.Error(), c.want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
