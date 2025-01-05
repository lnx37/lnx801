package main

import (
	_ "./lib/go-sqlite3"

	"database/sql"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"sort"
	"strings"
	"time"
)

var SETTINGS = struct {
	VERSION          string
	DATA_SOURCE_NAME string
	TOKEN            string
	DEBUG            bool
}{
	VERSION:          "20241031",
	DATA_SOURCE_NAME: "lnx801.db",
	TOKEN:            "123456",
	DEBUG:            false,
}

var (
	// //go:embed static/pure-min.css
	// STATIC embed.FS

	//go:embed template/index.html
	//go:embed template/detail.html
	//go:embed template/distribution.html
	TEMPLATE embed.FS
)

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

func Catch500(response http.ResponseWriter) {
	var err interface{}
	err = recover()
	if err != nil {
		log.Println(err)
		log.Println(string(debug.Stack()))
		Api(response, 500)
	}
}

func TimeTaken(started time.Time, action string) {
	var elapsed time.Duration
	elapsed = time.Since(started)
	log.Printf("%v took %v\n", action, elapsed)
}

// workaround, timezone issue
// // sql: problems with time.Time
// https://groups.google.com/g/golang-nuts/c/4ebvN6Bgv3M
// // fixed timezone problem for datetime types #155
// https://github.com/mattn/go-sqlite3/pull/155
func LocalizeTz(input time.Time) (time.Time, error) {
	var err error

	var x string
	x = input.Format("2006-01-02 15:04:05")

	var output time.Time
	output, err = time.ParseInLocation("2006-01-02 15:04:05", x, time.Local)

	return output, err
}

func MakeHandler(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		defer Catch500(response)
		defer TimeTaken(time.Now(), request.URL.Path)

		log.Println("request.URL.Path:", request.URL.Path)

		if strings.HasPrefix(request.URL.Path, "/api/report_") {
			var token string
			token = request.Header.Get("token")

			if token != SETTINGS.TOKEN {
				Api(response, 401)
			} else {
				next(response, request)
			}
		} else {
			next(response, request)
		}
	}
}

func Api(response http.ResponseWriter, code int, args ...interface{}) {
	var err error

	var data map[string]interface{}
	data = map[string]interface{}{
		"code": code,
		"msg":  http.StatusText(code),
	}

	if len(args) == 0 {
	} else if len(args) == 1 {
		data["data"] = args[0]
	} else if len(args) == 2 {
		data["data"] = args[0]

		var key string
		var value interface{}

		for key, value = range args[1].(map[string]interface{}) {
			data[key] = value
		}
	} else {
	}

	var body []byte
	body, err = json.Marshal(data)
	Raise(err)

	log.Println("code:", code)

	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	// https://github.com/golang/go/blob/go1.17.8/src/net/http/server.go#L2058
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(code)
	response.Write(body)
}

func HttpStatusOk(response http.ResponseWriter, request *http.Request) {
	response.WriteHeader(http.StatusOK)
}

func Index(response http.ResponseWriter, request *http.Request) {
	if !(request.URL.Path == "/" || strings.HasPrefix(request.URL.Path, "/index")) {
		Api(response, 404)
		return
	}

	var err error

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Raise(err)

	var query string
	query = `SELECT id, ip, mac, name, heartbeat_time FROM device`

	var rows *sql.Rows
	rows, err = db.Query(query)
	defer rows.Close()
	Raise(err)

	var devices []map[string]interface{}
	devices = make([]map[string]interface{}, 0)

	var now time.Time
	now = time.Now()

	for rows.Next() {
		var id int64
		var ip string
		var mac string
		var name string
		var heartbeat_time time.Time

		err = rows.Scan(&id, &ip, &mac, &name, &heartbeat_time)
		Raise(err)

		heartbeat_time, err = LocalizeTz(heartbeat_time)
		Skip(err)

		var heartbeat_time2 string
		heartbeat_time2 = heartbeat_time.Format("2006-01-02 15:04:05")

		var time_offset time.Duration
		var time_offset2 int
		time_offset = now.Sub(heartbeat_time)
		time_offset2 = int(time_offset.Seconds())

		devices = append(
			devices,
			map[string]interface{}{
				"id":             id,
				"ip":             ip,
				"mac":            mac,
				"name":           name,
				"heartbeat_time": heartbeat_time2,
				"time_offset":    time_offset2,
			},
		)
	}

	sort.Slice(devices, func(i int, j int) bool { return devices[i]["ip"].(string) < devices[j]["ip"].(string) })

	var data struct {
		Devices []map[string]interface{} `json:"devices"`
	}
	data.Devices = devices

	if strings.HasSuffix(request.URL.Path, ".json") {
		Api(response, 200, data)
	} else {
		var tpl *template.Template
		if SETTINGS.DEBUG {
			tpl, err = template.ParseFiles("template/index.html")
		} else {
			tpl, err = template.ParseFS(TEMPLATE, "template/index.html")
		}
		Skip(err)
		tpl.Execute(response, data)
	}
}

func Detail(response http.ResponseWriter, request *http.Request) {
	var err error

	var values url.Values
	values = request.URL.Query()
	log.Println("values:", values)

	var ip string
	ip = values.Get("ip")
	ip = strings.TrimSpace(ip)
	log.Println("ip:", ip)

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Raise(err)

	var now time.Time
	now = time.Now()

	var begin_time string
	begin_time = now.Add(-(time.Duration(1440) * time.Minute)).Format("2006-01-02 15:04:05")
	log.Println("begin_time:", begin_time)

	var end_time string
	end_time = now.Format("2006-01-02 15:04:05")
	log.Println("end_time:", end_time)

	var query string
	var rows *sql.Rows
	if ip != "" {
		query = `
			SELECT id, ip, mac, name, heartbeat_time
			FROM device_log
			WHERE ip=? AND heartbeat_time>=? AND heartbeat_time<=?
			ORDER BY heartbeat_time DESC
		`
		rows, err = db.Query(query, ip, begin_time, end_time)
	} else {
		query = `
			SELECT id, ip, mac, name, heartbeat_time
			FROM device_log
			WHERE heartbeat_time>=? AND heartbeat_time<=?
			ORDER BY heartbeat_time DESC
		`
		rows, err = db.Query(query, begin_time, end_time)
	}
	defer rows.Close()
	Raise(err)

	var device_logs []map[string]interface{}
	device_logs = make([]map[string]interface{}, 0)

	for rows.Next() {
		var id int64
		var ip string
		var mac string
		var name string
		var heartbeat_time time.Time

		err = rows.Scan(&id, &ip, &mac, &name, &heartbeat_time)
		Raise(err)

		var heartbeat_time2 string
		heartbeat_time2 = heartbeat_time.Format("2006-01-02 15:04:05")

		device_logs = append(
			device_logs,
			map[string]interface{}{
				"id":             id,
				"ip":             ip,
				"mac":            mac,
				"name":           name,
				"heartbeat_time": heartbeat_time2,
			},
		)
	}

	var data struct {
		DeviceLogs []map[string]interface{} `json:"device_logs"`
	}
	data.DeviceLogs = device_logs

	if strings.HasSuffix(request.URL.Path, ".json") {
		Api(response, 200, data)
	} else {
		var tpl *template.Template
		if SETTINGS.DEBUG {
			tpl, err = template.ParseFiles("template/detail.html")
		} else {
			tpl, err = template.ParseFS(TEMPLATE, "template/detail.html")
		}
		Skip(err)
		tpl.Execute(response, data)
	}
}

func Distribution(response http.ResponseWriter, request *http.Request) {
	var err error

	var values url.Values
	values = request.URL.Query()
	log.Println("values:", values)

	var ip string
	ip = values.Get("ip")
	ip = strings.TrimSpace(ip)
	log.Println("ip:", ip)

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Raise(err)

	var now time.Time
	now = time.Now()

	var begin_time string
	begin_time = fmt.Sprintf("%s 00:00:00", now.AddDate(0, 0, -30).Format("2006-01-02"))
	log.Println("begin_time:", begin_time)

	var end_time string
	end_time = fmt.Sprintf("%s 23:59:59", now.Format("2006-01-02"))
	log.Println("end_time:", end_time)

	var dates []string
	dates = make([]string, 0)
	{
		var i int
		for i = 0; i <= 30; i++ {
			dates = append(dates, now.AddDate(0, 0, -i).Format("20060102"))
		}
	}
	log.Println("dates:", dates)

	var query string
	var rows *sql.Rows
	if ip != "" {
		query = `
			SELECT id, ip, name, heartbeat_time
			FROM device_log
			WHERE ip=? AND heartbeat_time>=? AND heartbeat_time<=?
		`
		rows, err = db.Query(query, ip, begin_time, end_time)
	} else {
		query = `
			SELECT id, ip, name, heartbeat_time
			FROM device_log
			WHERE heartbeat_time>=? AND heartbeat_time<=?
		`
		rows, err = db.Query(query, begin_time, end_time)
	}
	defer rows.Close()
	Raise(err)

	var device_logs map[string]map[string]int
	device_logs = make(map[string]map[string]int)

	for rows.Next() {
		var id int64
		var ip string
		var name string
		var heartbeat_time time.Time

		err = rows.Scan(&id, &ip, &name, &heartbeat_time)
		Raise(err)

		var year_month_day string
		year_month_day = fmt.Sprintf("%d%02d%02d", heartbeat_time.Year(), int(heartbeat_time.Month()), heartbeat_time.Day())
		// log.Println("year_month_day:", year_month_day)

		var hour string
		hour = fmt.Sprintf("%02d", heartbeat_time.Hour())
		// log.Println("hour:", hour)

		var ok bool
		_, ok = device_logs[year_month_day]
		if ok {
			var ok2 bool
			_, ok2 = device_logs[year_month_day][hour]
			if ok2 {
				device_logs[year_month_day][hour] += 1
			} else {
				device_logs[year_month_day][hour] = 1
			}
		} else {
			device_logs[year_month_day] = map[string]int{hour: 1}
		}
	}
	log.Println("device_logs:", device_logs)

	var hours []string
	hours = []string{
		"00", "01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11",
		"12", "13", "14", "15", "16", "17", "18", "19", "20", "21", "22", "23",
	}

	var data struct {
		Dates      []string                  `json:"dates"`
		Hours      []string                  `json:"hours"`
		DeviceLogs map[string]map[string]int `json:"device_logs"`
	}
	data.Dates = dates
	data.Hours = hours
	data.DeviceLogs = device_logs
	log.Println("data:", data)

	if strings.HasSuffix(request.URL.Path, ".json") {
		Api(response, 200, data)
	} else {
		var tpl *template.Template
		if SETTINGS.DEBUG {
			tpl, err = template.ParseFiles("template/distribution.html")
		} else {
			tpl, err = template.ParseFS(TEMPLATE, "template/distribution.html")
		}
		Skip(err)
		tpl.Execute(response, data)
	}
}

func Report(response http.ResponseWriter, request *http.Request) {
	var err error

	var body []byte
	body, err = ioutil.ReadAll(request.Body)
	log.Println("body:", string(body))
	Raise(err)

	var data []map[string]interface{}
	json.Unmarshal(body, &data)

	log.Println("data:", data)

	if len(data) == 0 {
		Api(response, 400)
		return
	}

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Raise(err)

	var device map[string]interface{}
	for _, device = range data {
		log.Println("device:", device)

		var ip string
		var mac string
		var name string
		var heartbeat_time string

		ip = device["ip"].(string)
		mac = device["mac"].(string)
		name = device["name"].(string)
		heartbeat_time = device["heartbeat_time"].(string)

		// if ip != "192.168.18.107" {
		// 	continue
		// }

		{
			var query string
			query = `INSERT INTO device_log (ip, mac, name, heartbeat_time) VALUES (?,?,?,?)`
			_, err = db.Exec(query, ip, mac, name, heartbeat_time)
			Raise(err)
		}

		var rows_affected int64
		{
			var query string
			query = `UPDATE device set mac=?, name=?, heartbeat_time=? WHERE ip=?`

			var result sql.Result
			result, err = db.Exec(query, mac, name, heartbeat_time, ip)

			rows_affected, err = result.RowsAffected()
			log.Println("rows_affected:", rows_affected)
			Raise(err)
		}
		{
			if rows_affected == 0 {
				var query string
				query = `INSERT INTO device (ip, mac, name, heartbeat_time) VALUES (?,?,?,?)`
				_, err = db.Exec(query, ip, mac, name, heartbeat_time)
				Raise(err)
			}
		}
	}

	Api(response, 200)
}

func CreateTableDevice() {
	var err error

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Raise(err)

	var query string
	query = "SELECT 1 FROM device"

	var rows *sql.Rows
	rows, err = db.Query(query)
	if rows != nil {
		defer rows.Close()
	}
	Skip(err)

	if rows == nil {
		var query2 string
		query2 = `
			CREATE TABLE device (
				id             INTEGER PRIMARY KEY AUTOINCREMENT,
				ip             VARCHAR(100) NOT NULL,
				mac            VARCHAR(100) NOT NULL,
				name           VARCHAR(100) NOT NULL,
				heartbeat_time DATETIME     NOT NULL
			)
		`

		_, err = db.Exec(query2)
		Raise(err)

		log.Println("created table device")
	}
}

func CreateTableDeviceLog() {
	var err error

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Raise(err)

	var query string
	query = "SELECT 1 FROM device_log"
	query = fmt.Sprintf(query)

	var rows *sql.Rows
	rows, err = db.Query(query)
	if rows != nil {
		defer rows.Close()
	}
	Skip(err)

	if rows == nil {
		{
			var query2 string
			query2 = `
				CREATE TABLE device_log (
					id             INTEGER PRIMARY KEY AUTOINCREMENT,
					ip             VARCHAR(100) NOT NULL,
					mac            VARCHAR(100) NOT NULL,
					name           VARCHAR(100) NOT NULL,
					heartbeat_time DATETIME     NOT NULL
				)
			`
			query2 = fmt.Sprintf(query2)

			_, err = db.Exec(query2)
			Raise(err)
		}

		{
			var query2 string
			query2 = "CREATE INDEX idx__device_log__heartbeat_time ON device_log (heartbeat_time)"
			query2 = fmt.Sprintf(query2)
			_, err = db.Exec(query2)
			Raise(err)
		}

		log.Println("created table device_log")
	}
}

func InitDb() {
	CreateTableDevice()
	CreateTableDeviceLog()
}

func main() {
	defer Catch()

	var err error

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var host string
	var port int
	var debug bool
	// flag.StringVar(&host, "host", "0.0.0.0", "Host")
	flag.StringVar(&host, "host", "127.0.0.1", "Host")
	flag.IntVar(&port, "port", 801, "Port")
	flag.BoolVar(&debug, "debug", false, "Debug")
	flag.Parse()
	log.Println("host:", host)
	log.Println("port:", port)
	log.Println("debug:", debug)

	var address string
	// :1234, 0.0.0.0:1234, 127.0.0.1:1234
	address = fmt.Sprintf("%s:%d", host, port)
	log.Println("address:", address)

	SETTINGS.DEBUG = debug
	log.Printf("SETTINGS: %+v\n", SETTINGS)

	InitDb()

	http.HandleFunc("/", MakeHandler(Index))
	http.HandleFunc("/index", MakeHandler(Index))
	http.HandleFunc("/index.html", MakeHandler(Index))
	http.HandleFunc("/index.json", MakeHandler(Index))
	http.HandleFunc("/detail", MakeHandler(Detail))
	http.HandleFunc("/detail.html", MakeHandler(Detail))
	http.HandleFunc("/detail.json", MakeHandler(Detail))
	http.HandleFunc("/distribution", MakeHandler(Distribution))
	http.HandleFunc("/distribution.html", MakeHandler(Distribution))
	http.HandleFunc("/distribution.json", MakeHandler(Distribution))
	http.HandleFunc("/favicon.ico", MakeHandler(HttpStatusOk))
	http.HandleFunc("/api/report", MakeHandler(Report))

	// var httpFileSystem http.FileSystem
	// httpFileSystem = http.FS(STATIC)
	// var httpHandler http.Handler
	// httpHandler = http.FileServer(httpFileSystem)
	// http.Handle("/static/", httpHandler)

	log.Printf("ListenAndServe: http://%v/\n", address)
	err = http.ListenAndServe(address, nil)
	Raise(err)
}
