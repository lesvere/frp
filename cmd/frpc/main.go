// Copyright 2016 fatedier, fatedier@gmail.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	docopt "github.com/docopt/docopt-go"
	ini "github.com/vaughan0/go-ini"

	"github.com/fatedier/frp/client"
	"github.com/fatedier/frp/models/config"
	"github.com/fatedier/frp/utils/log"
	"github.com/fatedier/frp/utils/version"
)

var (
	configFile string = "./frpc.ini"
)

var usage string = `frpc is the client of frp

Usage: 
    frpc [-c config_file] [-L log_file] [--log-level=<log_level>] [--server-addr=<server_addr>]
    frpc [-c config_file] --reload
    frpc -h | --help
    frpc -v | --version

Options:
    -c config_file              set config file
    -L log_file                 set output log file, including console
    --log-level=<log_level>     set log level: debug, info, warn, error
    --server-addr=<server_addr> addr which frps is listening for, example: 0.0.0.0:7000
    --reload                    reload configure file without program exit
    -h --help                   show this screen
    -v --version                show version
`

func main() {
	var err error
	confFile := "./frpc.ini"
	// the configures parsed from file will be replaced by those from command line if exist
	args, err := docopt.Parse(usage, nil, true, version.Full(), false)

	if args["-c"] != nil {
		confFile = args["-c"].(string)
	}

	conf, err := ini.LoadFile(confFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	config.ClientCommonCfg, err = config.LoadClientCommonConf(conf)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	config.ClientCommonCfg.ConfigFile = confFile

	// check if reload command
	if args["--reload"] != nil {
		if args["--reload"].(bool) {
			req, err := http.NewRequest("GET", "http://"+
				config.ClientCommonCfg.AdminAddr+":"+fmt.Sprintf("%d", config.ClientCommonCfg.AdminPort)+"/api/reload", nil)
			if err != nil {
				fmt.Printf("frps reload error: %v\n", err)
				os.Exit(1)
			}

			authStr := "Basic " + base64.StdEncoding.EncodeToString([]byte(config.ClientCommonCfg.AdminUser+":"+
				config.ClientCommonCfg.AdminPwd))

			req.Header.Add("Authorization", authStr)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				fmt.Printf("frpc reload error: %v\n", err)
				os.Exit(1)
			} else {
				defer resp.Body.Close()
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					fmt.Printf("frpc reload error: %v\n", err)
					os.Exit(1)
				}
				res := &client.GeneralResponse{}
				err = json.Unmarshal(body, &res)
				if err != nil {
					fmt.Printf("http response error: %s\n", strings.TrimSpace(string(body)))
					os.Exit(1)
				} else if res.Code != 0 {
					fmt.Printf("reload error: %s\n", res.Msg)
					os.Exit(1)
				}
				fmt.Printf("reload success\n")
				os.Exit(0)
			}
		}
	}

	if args["-L"] != nil {
		if args["-L"].(string) == "console" {
			config.ClientCommonCfg.LogWay = "console"
		} else {
			config.ClientCommonCfg.LogWay = "file"
			config.ClientCommonCfg.LogFile = args["-L"].(string)
		}
	}

	if args["--log-level"] != nil {
		config.ClientCommonCfg.LogLevel = args["--log-level"].(string)
	}

	if args["--server-addr"] != nil {
		addr := strings.Split(args["--server-addr"].(string), ":")
		if len(addr) != 2 {
			fmt.Println("--server-addr format error: example 0.0.0.0:7000")
			os.Exit(1)
		}
		serverPort, err := strconv.ParseInt(addr[1], 10, 64)
		if err != nil {
			fmt.Println("--server-addr format error, example 0.0.0.0:7000")
			os.Exit(1)
		}
		config.ClientCommonCfg.ServerAddr = addr[0]
		config.ClientCommonCfg.ServerPort = serverPort
	}

	if args["-v"] != nil {
		if args["-v"].(bool) {
			fmt.Println(version.Full())
			os.Exit(0)
		}
	}

	pxyCfgs, visitorCfgs, err := config.LoadProxyConfFromFile(config.ClientCommonCfg.User, conf, config.ClientCommonCfg.Start)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	log.InitLog(config.ClientCommonCfg.LogWay, config.ClientCommonCfg.LogFile,
		config.ClientCommonCfg.LogLevel, config.ClientCommonCfg.LogMaxDays)

	svr := client.NewService(pxyCfgs, visitorCfgs)

	// Capture the exit signal if we use kcp.
	if config.ClientCommonCfg.Protocol == "kcp" {
		go HandleSignal(svr)
	}

	err = svr.Run()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func HandleSignal(svr *client.Service) {
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	svr.Close()
	time.Sleep(250 * time.Millisecond)
	os.Exit(0)
}
