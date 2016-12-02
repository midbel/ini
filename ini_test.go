package ini

import (
	"strings"
	"testing"
)

const account = `
[account]
user = "nobody"
passwd = "foobar"
enabled = true
`

const directory = `
[ldap]
host   = "ldap://localhost:389"
bind   = "cn=admin,dc=foobar,dc=com"
passwd = "helloworld"
base   = "dc=foobar,dc=com"
hash   = "md5"

[node.users]
;definition of node to lookup user's account
node  = "ou=users,dc=foobar,dc=be"
attr  = "uid"
scope = 2
class = "posixAccount"

[node.groups]
;definition of node to lookup user's groups
node  = "ou=groups,dc=foobar,dc=be"
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

[urls.group0]
group  = "udp://224.0.0.1:11001"
prefix = "0xbeef"
size   = 1024
keep   = false

[urls.group1]
group  = "udp://224.0.0.1:11002"
prefix = "0xdead"
size   = 1024
keep   = false

[urls.group2]
group  = "udp://224.0.0.1:11003"
prefix = "0xcafe"
size   = 1024
keep   = false
`

func TestReadAccount(t *testing.T) {
	r := NewReader(strings.NewReader(account))
	r.Default = "account"
	if err := r.Read(nil); err != nil {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestReadDirectory(t *testing.T) {
	r := NewReader(strings.NewReader(directory))
	r.Default = "ldap"
	if err := r.Read(nil); err != nil {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestReadURLS(t *testing.T) {
	r := NewReader(strings.NewReader(urls))
	r.Default = "urls"
	if err := r.Read(nil); err != nil {
		t.Errorf("unexpected error: %s", err)
	}
}