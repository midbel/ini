package ini

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

const quote = `
[quote]
expr = "var0 == \"value1\" && var1 == \"value2\""
`

const account = `
[account]
user = "nobody"
passwd = "foobar"
enabled = true
alias   = ["root", "nobody", "test",]
`

const directory = `
[ldap]
host   = "ldap://localhost:389"
bind   = "cn=admin,dc=foobar,dc=com"
passwd = "helloworld"
base   = "dc=foobar,dc=com"
hash   = "md5"

[users]
;definition of node to lookup user's account
node  = "ou=users,dc=foobar,dc=com"
attr  = "uid"
scope = 2
class = "posixAccount"

[groups]
;definition of node to lookup user's groups
node  = "ou=groups,dc=foobar,dc=com"
attr  = "cn"
scope = 2
class = "posixGroup"
`

const urls = `
[urls]
addr = "tcp://localhost:6789"
verbose = true
count = 5
size = 1024
datadir = "/var/tmp/"

[groups.group0]
group  = "udp://224.0.0.1:11001"
prefix = "0xbeef"
size   = 1024
keep   = false

[groups.group1]
group  = "udp://224.0.0.1:11002"
prefix = "0xdead"
size   = 1024
keep   = false

[groups.group2]
group  = "udp://224.0.0.1:11003"
prefix = "0xcafe"
size   = 1024
keep   = false
`
const timestamp = `
[timestamp]
before = "1970-01-01T00:00:00Z"
after = "2020-01-01T00:00:00Z"
`

func ExampleReader() {
	r := NewReader(strings.NewReader(account))
	r.Default = "account"

	a := struct {
		User    string
		Passwd  string
		Enabled bool
	}{}

	r.Read(&a)
	fmt.Printf("%+v\n", a)
	//Output:
	//{User:nobody Passwd:foobar Enabled:true}
}

func TestReadUnmarshalText(t *testing.T) {
	r := NewReader(strings.NewReader(timestamp))
	r.Default = "timestamp"

	c := struct {
		Before time.Time
		After  time.Time
	}{}
	if err := r.Read(&c); err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if c.Before.IsZero() || c.After.IsZero() {
		t.Errorf("before/after fields not correctly unmarshaled")
	}
	t.Logf("%+v", c)
}

func TestReadSection(t *testing.T) {
	type Directory struct {
		Host   string
		Bind   string
		Passwd string
		Base   string
		Hash   string
	}
	type Account struct {
		User    string
		Passwd  string
		Enabled bool
		Alias   []string
	}

	data := []struct {
		Section string
		Sample  string
		Data    interface{}
		Found   bool
	}{
		{"account", account, new(Account), true},
		{"directory", account, new(Directory), false},
		{"ldap", directory, new(Directory), true},
	}
	for i, d := range data {
		r := NewReader(strings.NewReader(d.Sample))
		err := r.ReadSection(d.Section, d.Data)
		switch {
		case d.Found && err != nil:
			t.Errorf("#%d: fail to read section %s", i, d.Section)
		case !d.Found && err == nil:
			t.Errorf("#%d: section %s found and should not", i, d.Section)
		}
	}
}

func TestReadQuote(t *testing.T) {
	r := NewReader(strings.NewReader(quote))
	r.Default = "quote"

	d := struct {
		Expr string
	}{}
	if err := r.Read(&d); err != nil {
		t.Errorf("unexpected error: %s", err)
		t.FailNow()
	}
	result := `var0 == "value1" && var1 == "value2"`
	if strings.TrimSpace(d.Expr) != strings.TrimSpace(result) {
		t.Errorf("expected %s, got %s", result, d.Expr)
	}
}

func TestReadAccount(t *testing.T) {
	r := NewReader(strings.NewReader(account))
	r.Default = "account"

	a := struct {
		User    string
		Passwd  string
		Enabled bool
		Alias   []string
	}{"root", "helloworld", false, make([]string, 0)}
	if err := r.Read(&a); err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if a.User != "nobody" {
		t.Errorf("user: expected nobody; got %s", a.User)
	}
	if a.Passwd != "foobar" {
		t.Errorf("passwd: expected nobody; got %s", a.Passwd)
	}
	if !a.Enabled {
		t.Errorf("enabled: expected true; got %t", a.Enabled)
	}
	if len(a.Alias) == 0 {
		t.Errorf("empty alias array, expected 3")
	}
	t.Logf("%+v", a)
}

func TestReadDirectory(t *testing.T) {
	type Node struct {
		Node  string
		Attr  string
		Class string
		Scope int
	}
	type Directory struct {
		Host   string
		Bind   string
		Passwd string
		Base   string
		Hash   string
		Users  Node
		Groups Node
	}
	d := Directory{}
	r := NewReader(strings.NewReader(directory))
	r.Default = "ldap"
	if err := r.Read(&d); err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if d.Users.Node != "ou=users,dc=foobar,dc=com" {
		t.Errorf("users.node: expected ou=users,dc=foobar,dc=com; got %s", d.Users.Node)
	}
	if d.Groups.Node != "ou=groups,dc=foobar,dc=com" {
		t.Errorf("groups.node: expected ou=groups,dc=foobar,dc=com; got %s", d.Users.Node)
	}
	t.Logf("%+v", d)
}

func TestReadURLS(t *testing.T) {
	type Group struct {
		Group  string
		Prefix string
		Keep   bool
		Size   int
	}

	type Multiplex struct {
		Addr    string
		Verbose bool
		Count   int
		Size    int
		Datadir string
		Groups  []Group
	}
	m := Multiplex{}
	r := NewReader(strings.NewReader(urls))
	r.Default = "urls"
	if err := r.Read(&m); err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if len(m.Groups) == 0 {
		t.Errorf("empty groups; expected length of 3")
	}
	for i, g := range m.Groups {
		if g.Group == "" {
			t.Errorf("empty group name at %d, expected url", i)
		}
	}
	t.Logf("%+v\n", m)
}
