package ini

import (
	"strings"
	"testing"
)

var sample = `
[sample]
user = "nobody"
passwd = "foobar"
enabled = true
`

func TestReadBasic(t *testing.T) {
	u := struct{
		User    string `ini:"sample>user"`
		Passwd  string `ini:"sample>passwd"`
		Enabled bool   `ini:"-"`
	}{}
	if err := Read(strings.NewReader(sample), &u); err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if u.User != "nobody" {
		t.Errorf("user: expected nobody, got %s", u.User)
	}
	if u.Passwd != "foobar" {
		t.Errorf("passwd: expected foobar, got %s", u.User)
	}
	if u.Enabled != false {
		t.Error("enabled: expected false, got true")
	}
}
