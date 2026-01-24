package transport

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	readDeadline  = 10 * time.Second
	writeDeadline = 30 * time.Second
	fetchTimeout  = 10 * time.Second
)

var httpClient = &http.Client{
	Timeout: fetchTimeout,
}

func Fake(conn net.Conn, fakeURL string) error {
	defer conn.Close()
	
	conn.SetReadDeadline(time.Now().Add(readDeadline))
	conn.SetWriteDeadline(time.Now().Add(writeDeadline))
	
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\r\n" || line == "\n" {
			break
		}
	}
	
	response, err := fetch(fakeURL)
	if err != nil {
		return err
	}
	
	_, err = conn.Write(response)
	return err 
}

func fetch(domain string) ([]byte, error) {
	resp, err := httpClient.Get("https://" + domain)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	var response strings.Builder
	response.WriteString(fmt.Sprintf("HTTP/1.1 %d %s\r\n", resp.StatusCode, http.StatusText(resp.StatusCode)))
	
	for key, values := range resp.Header {
		for _, value := range values {
			response.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
		}
	}
	response.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(body)))
	response.WriteString("Connection: close\r\n")
	response.WriteString("\r\n")
	response.Write(body)
	
	return []byte(response.String()), nil
}
