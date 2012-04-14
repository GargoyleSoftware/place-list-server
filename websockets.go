package main

import (
	"code.google.com/p/go.net/websocket"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

func main() {

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<html>
<head>
<script src="http://code.jquery.com/jquery-1.7.2.min.js"></script>
<script>
$(document).ready(function() {
    var conn = new WebSocket("ws://localhost:8080/socket");
    conn.onclose = function(event) {
        console.log({"close": event});
    };
    conn.onmessage = function(event) {
        $("#poop").append(event.data);
    };
});
</script>
</head>
<body>
<p>this too shall pass</p>
<ul id="poop"></ul>
</body>
</html>`)
	})

	http.Handle("/socket", websocket.Handler(func(sock *websocket.Conn) {
		for {
			io.WriteString(sock, fmt.Sprintf("<li>%d</li>", rand.Int63n(1e9)))
			time.Sleep(time.Duration(5e6))
		}
	}))

	http.ListenAndServe(":8080", nil)
}
