package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type CommandOutput struct {
	Command string
	Output  string
}

func updateOutputs() []CommandOutput {
	var outputs = []CommandOutput{
		{Command: "nvidia-smi"},
		{Command: "free -h"},
		{Command: `cpufreq-info | grep 'current CPU' | awk '{  		
			freq_width = 5  
			printf "CPU%s: %-*.*s %s\t", NR, freq_width, freq_width, $5 $6, ($7 != "" ? $7 : "")  
			if (NR % 4 == 0) {  
				printf "\n"  
			}  
		}  
		END {  
			if (NR % 4 != 0) {  
				printf "\n"  
			}  
		}'
		`},
		{Command: "sensors"},
		{Command: "tail -n 5 ~/Backup/backup-log.txt"},
	}
outer:
	for index, cmdName := range outputs {
		if strings.Contains(cmdName.Command, "|") {
			cmds := strings.Split(cmdName.Command, "|")
			var tbuf bytes.Buffer
			var err error
			for _, cmdstr := range cmds {
				tbuf, err = cmd(tbuf, cmdstr)
				if err != nil {
					continue outer
				}
			}
			outputs[index] = CommandOutput{Command: cmdName.Command, Output: tbuf.String()}
			continue
		}

		buf, err := cmd(bytes.Buffer{}, cmdName.Command)
		if err != nil {
			continue
		}
		outputs[index] = CommandOutput{Command: cmdName.Command, Output: buf.String()}

	}
	return outputs
}
func cmd(inbuf bytes.Buffer, cmdName string) (outbuf bytes.Buffer, err error) {
	cmd := exec.Command("bash", "-c", cmdName)
	cmd.Stdin = &inbuf
	cmd.Stdout = &outbuf
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error running command %s: %v\n", cmdName, err)
		return outbuf, err
	}
	return outbuf, nil
}

func update(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Connection", "keep-alive")
	c.Header("Cache-Control", "no-cache")
	c.Header("Retry", "5000")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.AbortWithError(http.StatusInternalServerError, fmt.Errorf(""))
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			tmpl := template.Must(template.New("status").Parse(`
			{{ range .Outputs }}
            <p>                
                <pre>{{ .Output }}</pre>
            </p>
            {{ end }}
			`))
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, map[string]any{"Outputs": updateOutputs()}); err != nil {
				c.AbortWithError(http.StatusInternalServerError, err)
				return
			}
			c.SSEvent("message", buf.String())
			flusher.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}

}
func statusPage(c *gin.Context) {
	tmpl := template.Must(template.New("status_layout").Parse(`
	<!DOCTYPE html>
	<html lang="en">
	
	<head>
		<meta charset="UTF-8">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<title>Status</title>
		<style>
			body {
				background-color: black;
			}
	
			div {
				color: beige;
				columns: 2;
				column-gap: 150px;
				padding-left:50px;
			}
	
			p {
	
				margin-right: 20px;
			}
		</style>
	</head>
	
	<body>
		<div id="sse-updates">
		</div>
		<script>
			var source = new EventSource("/update");
			source.onmessage = function (event) {
				document.getElementById('sse-updates').innerHTML = event.data;
			};
	
			source.onerror = function (event) {
				if (event.readyState == EventSource.CLOSED) {
					console.log("Connection was closed.");
				}
			};
		</script>
	</body>
	
	</html>
	`))
	tmpl.Execute(c.Writer, map[string]any{"Outputs": updateOutputs()})
}
func main() {
	r := gin.Default()
	r.GET("/", statusPage)
	r.GET("/update", update)
	r.Run("127.0.0.1:9999")
}
