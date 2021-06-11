package server

import (
	"fmt"
	"gingle-rpc/service"
	"html/template"
	"io"
	"net/http"
)

const debugHtml = `
<html>
	<body>
	<title>GingleRPC Services</title>
	{{range .}}
	<hr>
	Service {{.Name}}
	<hr>
		<table>
		<th align=center>Method</th><th align=center>Calls</th>
		{{range $name, $mtype := .Method}}
			<tr>
			<td align=left font=fixed>{{$name}}({{$mtype.ArgType}}, {{$mtype.ReplyType}}) error</td>
			<td align=center>{{$mtype.NumCalls}}</td>
			</tr>
		{{end}}
		</table>
	{{end}}
	</body>
</html>
`

var debugTmpl *template.Template = template.Must(template.New("RPC Debug").Parse(debugHtml))

// DebugServer includes server
type DebugServer struct {
	*Server
}

// DebugDto includes name and rpc methods
type DebugDto struct {
	Name       string
	RpcMethods map[string]*service.RpcMethod
}

// ServeHTTP is to parse and execute the debug template
func (s *DebugServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var dtos []DebugDto
	s.Services.Range(func(nameInterface, svcInterface interface{}) bool {
		name := nameInterface.(string)
		svc := svcInterface.(*service.Service)
		dtos = append(dtos, DebugDto{
			Name:       name,
			RpcMethods: svc.RpcMethods,
		})
		return true
	})

	err := debugTmpl.Execute(w, dtos)
	if err != nil {
		io.WriteString(w, fmt.Sprintf("debug: failed to serve http, err: %v\n", err))
	}
}
