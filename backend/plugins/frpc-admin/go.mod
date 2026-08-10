module portalt-plugins/frpc-admin

go 1.26.5

require (
	github.com/BurntSushi/toml v1.5.0
	github.com/stretchr/testify v1.11.1
	google.golang.org/grpc v1.83.0
	gopkg.in/ini.v1 v1.67.0
)

require (
	golang.org/x/crypto v0.54.0
	portalt v0.0.0-00010101000000-000000000000
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace portalt => ../../../backend
