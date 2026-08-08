module github.com/Tonyjh07/PortalT-plugin-template

go 1.26.5

// 作为 submodule 置于 PortalT 仓库的 backend/plugins/<id>/ 下时，
// portalt/proto/plugin/v1 经 replace 解析到 PortalT 根模块。
require (
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.11 // indirect
)

require portalt v0.0.0-00010101000000-000000000000

require (
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
)

replace portalt => ../../../backend
