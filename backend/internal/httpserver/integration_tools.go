package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"k8s-aiops.local/backend/internal/mcpserver"
)

// listIntegrationTools publishes the same closed catalogue as the stdio gateway.
// The Web endpoint does not start a gateway or imply a connected MCP client.
func listIntegrationTools(c *gin.Context) {
	items := make([]gin.H, 0)
	for _, tool := range mcpserver.DefaultTools() {
		args := make([]gin.H, 0)
		for _, arg := range tool.Args {
			args = append(args, gin.H{"name": arg.Name, "required": arg.Required, "description": arg.Description})
		}
		items = append(items, gin.H{"name": tool.Name, "description": tool.Description, "method": "GET", "path": tool.Path, "arguments": args})
	}
	c.JSON(http.StatusOK, gin.H{"transport": "stdio", "read_only": true, "items": items})
}

// validateIntegrationTool exercises the gateway's own validator, without a
// backend client or executor. A passing call proves parameter validity only.
func validateIntegrationTool(c *gin.Context) {
	var input struct {
		Tool      string            `json:"tool"`
		Arguments map[string]string `json:"arguments"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_TOOL_REQUEST", "expected a tool name and string arguments")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeError(c, http.StatusBadRequest, "INVALID_TOOL_REQUEST", "expected a single JSON object")
		return
	}
	for _, tool := range mcpserver.DefaultTools() {
		if tool.Name != input.Tool {
			continue
		}
		accepted, err := tool.ValidateArguments(input.Arguments)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"allowed": false, "executed": false, "reason": err.Error()})
			return
		}
		path, err := tool.URLPath(accepted)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"allowed": false, "executed": false, "reason": err.Error()})
			return
		}
		query := url.Values{}
		for _, key := range tool.Query {
			if value, ok := accepted[key]; ok {
				query.Set(key, value)
			}
		}
		if len(query) > 0 {
			path += "?" + query.Encode()
		}
		c.JSON(http.StatusOK, gin.H{"allowed": true, "executed": false, "method": "GET", "path": path, "arguments": accepted})
		return
	}
	c.JSON(http.StatusOK, gin.H{"allowed": false, "executed": false, "reason": "tool is not in the published read-only catalogue"})
}
