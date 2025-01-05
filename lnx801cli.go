package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/netip"
	"os/exec"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

var SETTINGS = struct {
	VERSION string
	DEBUG   bool
	API     string
	TOKEN   string
}{
	VERSION: "20241031",
	DEBUG:   false,
	API:     "http://127.0.0.1:801/api",
	TOKEN:   "123456",
}

func Skip(err error) {
	if err != nil {
		log.Println(err)
		log.Println("skip error")
	}
}

func Raise(err error) {
	if err != nil {
		panic(err)
	}
}

func Catch() {
	var err interface{}
	err = recover()
	if err != nil {
		log.Println(err)
		log.Println(string(debug.Stack()))
	}
}

func TimeTaken(started time.Time, action string) {
	var elapsed time.Duration
	elapsed = time.Since(started)
	log.Printf("%v took %v\n", action, elapsed)
}

func ExecCmd(command string) (string, error) {
	var err error

	var cmd *exec.Cmd
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd = exec.Command("sh", "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Start()

	err = cmd.Wait()

	var output string
	if err == nil {
		output = stdout.String()
	} else {
		output = stderr.String()
	}

	return output, err
}

func ExecCmdWithTimeout(command string, args ...time.Duration) (string, error) {
	var err error

	var duration time.Duration
	duration = 10
	if len(args) == 1 {
		duration = args[0]
	}

	var cmd *exec.Cmd
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd = exec.Command("sh", "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Start()

	var done chan error
	done = make(chan error)
	go func() { done <- cmd.Wait() }()

	var timeout <-chan time.Time
	timeout = time.After(duration * time.Second)

	select {
	case <-timeout:
		cmd.Process.Kill()
		return "", errors.New(fmt.Sprintf("command timed out after %d secs", duration))
	case err = <-done:
		var output string
		if err == nil {
			output = stdout.String()
		} else {
			output = stderr.String()
		}
		return output, err
	}
}

func HttpPost(api string, data []byte) int64 {
	defer Catch()
	defer TimeTaken(time.Now(), api)

	var err error

	var request *http.Request
	request, err = http.NewRequest("POST", api, bytes.NewReader(data))
	Raise(err)

	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("token", SETTINGS.TOKEN)

	var client *http.Client
	client = &http.Client{Timeout: 30 * time.Second}

	var response *http.Response
	response, err = client.Do(request)
	if response != nil {
		defer response.Body.Close()
	}
	Raise(err)

	var http_status_code int64
	if response != nil {
		log.Println("response status:", response.Status)
		log.Println("response headers:", response.Header)

		var body []byte
		body, err = ioutil.ReadAll(response.Body)
		log.Println("response body:", string(body))
		Raise(err)

		http_status_code = int64(response.StatusCode)
	}

	return http_status_code
}

func GetCurrentTime() string {
	var current_time string
	current_time = time.Now().Format("2006-01-02 15:04:05")
	return current_time
}

func Cidr2Ips(cidr string) ([]string, error) {
	var err error

	var ips []string

	var prefix netip.Prefix
	prefix, err = netip.ParsePrefix(cidr)

	if err == nil {
		// 192.168.1.100/24 -> 192.168.1.0/24
		prefix = prefix.Masked()

		var addr netip.Addr
		addr = prefix.Addr()

		for {
			if !prefix.Contains(addr) {
				break
			}
			ips = append(ips, addr.String())
			addr = addr.Next()
		}
	}

	// // remove 192.168.1.0, 192.168.1.255
	// if len(ips) >=2 {
	// 	ips = ips[1 : len(ips)-1]
	// }

	return ips, err
}

func GetMacs() map[string]string {
	var err error

	var macs map[string]string
	macs = make(map[string]string)

	var cmd string
	var cmd_result string

	// cmd = fmt.Sprintf("arp -n")
	cmd = fmt.Sprintf(`arp -n |grep "^1" |grep -v "incomplete"`)
	cmd_result, err = ExecCmdWithTimeout(cmd)

	log.Println("cmd:", cmd)
	log.Println("cmd_result:", cmd_result)
	Skip(err)

	var lines []string
	lines = strings.Split(cmd_result, "\n")

	var line string
	for _, line = range lines {
		var fields []string
		fields = strings.Fields(line)

		if len(fields) >= 5 {
			var ip string
			var mac string
			ip = fields[0]
			mac = fields[2]

			macs[ip] = mac
		}
	}

	return macs
}

func PingIp(ip string) (string, error) {
	var err error

	var cmd string
	var cmd_result string

	// cmd = "ping -W1 -c1 192.168.18.107"
	cmd = fmt.Sprintf("ping -W1 -c1 %s", ip)
	cmd_result, err = ExecCmdWithTimeout(cmd)

	log.Println("ip:", ip, "cmd:", cmd)
	log.Println("ip:", ip, "cmd_result:", cmd_result)
	Skip(err)

	return cmd_result, err
}

func NslookupIp(ip string) (string, error) {
	var err error

	var cmd string
	var cmd_result string

	// cmd = fmt.Sprintf("nmap -sL %s", ip)
	cmd = fmt.Sprintf("nslookup %s", ip)
	cmd_result, err = ExecCmdWithTimeout(cmd)

	log.Println("ip:", ip, "cmd:", cmd)
	log.Println("ip:", ip, "cmd_result:", cmd_result)
	Skip(err)

	var device_name string
	// device_name = "unknown"
	device_name = ""
	if err == nil {
		if strings.Contains(cmd_result, "name = ") {
			// device_name = cmd_result
			var fields []string
			fields = strings.Split(cmd_result, "=")
			if len(fields) >= 2 {
				device_name = strings.TrimSpace(fields[1])
				var fields2 []string
				fields2 = strings.Split(device_name, ".")
				if len(fields2) >= 1 {
					device_name = strings.TrimSpace(fields2[0])
				}
			}
		}
	}
	log.Println("device_name:", device_name)

	return device_name, err
}

func GetDevices(ips []string) []map[string]interface{} {
	defer Catch()

	var macs map[string]string
	macs = GetMacs()
	log.Println("macs:", macs)

	// var chs = make(chan ch, 254)
	// var chs = make(chan ch, 256)
	// var chs = make(chan ch, 1024)
	var chs = make(chan map[string]interface{}, len(ips))
	{
		var wg sync.WaitGroup

		var ip string
		for _, ip = range ips {
			log.Println("ip:", ip)

			wg.Add(1)

			go func(ip string) {
				defer wg.Done()

				var err error

				var result string
				result, err = PingIp(ip)

				chs <- map[string]interface{}{
					"ip":     ip,
					"result": result,
					"err":    err,
				}
			}(ip)
		}

		wg.Wait()
		close(chs)
	}

	var targets []string
	targets = make([]string, 0)
	{
		var ch map[string]interface{}
		for ch = range chs {
			if ch["err"] == nil {
				log.Println("ch:", ch)
				targets = append(targets, ch["ip"].(string))
			}
		}
		log.Println("targets:", targets)
	}

	var devices []map[string]interface{}
	devices = make([]map[string]interface{}, 0)
	{
		var target string
		for _, target = range targets {
			log.Println("target:", target)

			var ip string
			var mac string
			var result string

			ip = target
			mac = macs[ip]
			result, _ = NslookupIp(ip)

			devices = append(
				devices,
				map[string]interface{}{
					"ip":             ip,
					"mac":            mac,
					"name":           result,
					"heartbeat_time": GetCurrentTime(),
				},
			)
		}
		log.Println("devices:", devices)
	}

	return devices
}

func main() {
	reflect.TypeOf(0)

	var err error

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var cidr string
	var host string
	var port int
	var debug bool
	// flag.StringVar(&cidr, "cidr", "192.168.18.0/16", "CIDR")
	flag.StringVar(&cidr, "cidr", "192.168.18.0/24", "CIDR")
	flag.StringVar(&host, "host", "127.0.0.1", "Host")
	flag.IntVar(&port, "port", 801, "Port")
	flag.BoolVar(&debug, "debug", false, "Debug")
	flag.Parse()
	log.Println("cidr:", cidr)
	log.Println("host:", host)
	log.Println("port:", port)
	log.Println("debug:", debug)

	SETTINGS.API = fmt.Sprintf("http://%s:%d/api", host, port)
	SETTINGS.DEBUG = debug
	log.Printf("SETTINGS: %+v\n", SETTINGS)

	var ips []string
	ips, err = Cidr2Ips(cidr)
	if err != nil {
		Raise(err)
	}
	log.Println("ips:", ips)

	for {
		var devices []map[string]interface{}
		devices = GetDevices(ips)

		var device map[string]interface{}
		for _, device = range devices {
			log.Println("device:", device)
		}

		var devices2 []byte
		devices2, err = json.Marshal(devices)
		log.Println("devices:", string(devices2))
		Raise(err)

		var api string
		api = fmt.Sprintf("%s/report", SETTINGS.API)
		log.Println("api:", api)
		log.Println("data:", string(devices2))
		HttpPost(api, devices2)

		if SETTINGS.DEBUG {
			time.Sleep(5 * time.Second)
		} else {
			time.Sleep(1 * time.Minute)
		}
	}
}
