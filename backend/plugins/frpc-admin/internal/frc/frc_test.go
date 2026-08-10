package frc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleINI = `# frp 客户端配置
[common]
server_addr = 127.0.0.1
server_port = 7000
token = demo-token

[ssh]
type = tcp
local_ip = 127.0.0.1
local_port = 22
remote_port = 6000
use_encryption = true

[web]
type = http
local_port = 8080
custom_domains = app.example.com, www.example.com
`

const sampleTOML = `serverAddr = "127.0.0.1"
serverPort = 7000

[auth]
token = "demo-token"

[[proxies]]
name = "ssh"
type = "tcp"
localIP = "127.0.0.1"
localPort = 22
remotePort = 6000
useEncryption = true

[[proxies]]
name = "web"
type = "http"
localPort = 8080
customDomains = ["app.example.com", "www.example.com"]
`

func TestDetect(t *testing.T) {
	assert.Equal(t, FormatINI, Detect([]byte(sampleINI)))
	assert.Equal(t, FormatTOML, Detect([]byte(sampleTOML)))
	assert.Equal(t, FormatTOML, Detect([]byte(`serverAddr = "x"`)))
	assert.Equal(t, FormatINI, Detect([]byte(`[myproxy]`)))
	assert.Equal(t, FormatINI, Detect([]byte("")))
}

func TestParseINI(t *testing.T) {
	c, err := Parse([]byte(sampleINI), string(FormatAuto))
	require.NoError(t, err)
	assert.Equal(t, FormatINI, c.Format)
	assert.Equal(t, "127.0.0.1", c.Server.ServerAddr)
	assert.Equal(t, 7000, c.Server.ServerPort)
	assert.Equal(t, "demo-token", c.Server.Token)
	require.Len(t, c.Proxies, 2)

	ssh := c.Proxies[0]
	assert.Equal(t, "ssh", ssh.Name)
	assert.Equal(t, "tcp", ssh.Type)
	assert.Equal(t, "127.0.0.1", ssh.LocalIP)
	assert.Equal(t, 22, ssh.LocalPort)
	assert.Equal(t, 6000, ssh.RemotePort)
	assert.Equal(t, "true", ssh.Extra["use_encryption"])

	web := c.Proxies[1]
	assert.Equal(t, []string{"app.example.com", "www.example.com"}, web.CustomDomains)
}

func TestParseTOML(t *testing.T) {
	c, err := Parse([]byte(sampleTOML), string(FormatAuto))
	require.NoError(t, err)
	assert.Equal(t, FormatTOML, c.Format)
	assert.Equal(t, "127.0.0.1", c.Server.ServerAddr)
	assert.Equal(t, 7000, c.Server.ServerPort)
	assert.Equal(t, "demo-token", c.Server.Token)
	require.Len(t, c.Proxies, 2)

	ssh := c.Proxies[0]
	assert.Equal(t, "ssh", ssh.Name)
	assert.Equal(t, 6000, ssh.RemotePort)
	assert.Equal(t, true, ssh.Extra["useEncryption"])

	web := c.Proxies[1]
	assert.Equal(t, []string{"app.example.com", "www.example.com"}, web.CustomDomains)
}

func TestRoundTripINI(t *testing.T) {
	c, err := Parse([]byte(sampleINI), string(FormatAuto))
	require.NoError(t, err)
	// 修改一个代理，未知键应保留
	c.Proxies[0].RemotePort = 7000
	c.Server.ServerAddr = "10.0.0.5"

	out, err := c.Render()
	require.NoError(t, err)

	c2, err := Parse(out, "ini")
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.5", c2.Server.ServerAddr)
	assert.Equal(t, 7000, c2.Proxies[0].RemotePort)
	assert.Equal(t, "true", c2.Proxies[0].Extra["use_encryption"], "未知键应保留")
	assert.Equal(t, []string{"app.example.com", "www.example.com"}, c2.Proxies[1].CustomDomains)
	// 代理顺序保留
	assert.Equal(t, []string{"ssh", "web"}, []string{c2.Proxies[0].Name, c2.Proxies[1].Name})
}

func TestRoundTripTOML(t *testing.T) {
	c, err := Parse([]byte(sampleTOML), string(FormatAuto))
	require.NoError(t, err)
	c.Proxies[0].LocalPort = 2222
	c.Server.Token = "new-token"

	out, err := c.Render()
	require.NoError(t, err)

	c2, err := Parse(out, "toml")
	require.NoError(t, err)
	assert.Equal(t, "new-token", c2.Server.Token)
	assert.Equal(t, 2222, c2.Proxies[0].LocalPort)
	assert.Equal(t, true, c2.Proxies[0].Extra["useEncryption"], "未知键应保留")
	assert.Equal(t, []string{"app.example.com", "www.example.com"}, c2.Proxies[1].CustomDomains)
}

func TestSyntaxCheck(t *testing.T) {
	assert.NoError(t, SyntaxCheck([]byte(sampleINI), "auto"))
	assert.NoError(t, SyntaxCheck([]byte(sampleTOML), "auto"))
	assert.Error(t, SyntaxCheck([]byte("[common"), "auto"), "未闭合段应报错")
	assert.Error(t, SyntaxCheck([]byte(""), "auto"))
	assert.Error(t, SyntaxCheck([]byte("   \n"), "auto"))
	assert.Error(t, SyntaxCheck([]byte("serverAddr = [broken"), "toml"))
}

func TestParseErrors(t *testing.T) {
	_, err := Parse([]byte("[common]\nserver_port = "), "ini")
	assert.NoError(t, err) // ini 宽松：server_port 空值忽略，不算错

	_, err = Parse([]byte("[common"), "ini")
	assert.Error(t, err, "未闭合段应报错")

	_, err = Parse([]byte("serverAddr = [broken"), "toml")
	assert.Error(t, err)

	_, err = Parse([]byte("whatever"), "bogus")
	assert.Error(t, err, "非法格式标识")
}
