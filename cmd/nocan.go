package main

import (
	"fmt"
	//"io/ioutil"
	"flag"
	"net/http"
	"pannetrat.com/nocan"
	"pannetrat.com/nocan/clog"
	"pannetrat.com/nocan/controller"
	"pannetrat.com/nocan/model"
	"strings"
)

type multiString []string

func (d *multiString) String() string {
	return fmt.Sprintf("%v", *d)
}

func (d *multiString) Set(value string) error {
	for _, str := range strings.Split(value, ",") {
		*d = append(*d, str)
	}
	return nil
}

var (
	optDeviceStrings multiString
	optChannels      multiString
	optLogTask       bool
)

func init() {
	flag.Var(&optDeviceStrings, "interface", "Interface to connect to (may be repeated)")
	flag.BoolVar(&optLogTask, "log-task", false, "Add a logging task (helps debug)")
	flag.Var(&optChannels, "channel", "Register a channel (may be repeated)")
}

func main() {
	//var id [8]byte

	flag.Parse()

	clog.Debug("Start")

	main := controller.NewApplication()
	//model.StringToUdid("01:02:03:04:05:06:07:88", id[:])
	//main.Nodes.Model.Register(id[:])

	if len(optDeviceStrings) > 0 {
		for _, itr := range optDeviceStrings {
			device, err := model.NewInterface(itr)
			if err != nil {
				return
			}
			main.Interfaces.Model.Add(device)
		}
	} else {
		clog.Warning("No interface was specified! Not much to do here.")
	}

	for _, itr := range optChannels {
		main.Channels.Model.Register(itr)
	}

	if optLogTask {
		lt := nocan.NewLogTask(main)
		if lt != nil {
			go lt.Run()
		}
	}

	homepage := controller.NewHomePageController()

	main.Router.GET("/api/channels", main.Channels.Index)
	main.Router.GET("/api/channels/*channel", main.Channels.Show)
	main.Router.PUT("/api/channels/*channel", main.Channels.Update)
	main.Router.GET("/api/nodes", main.Nodes.Index)
	main.Router.GET("/api/nodes/:node", main.Nodes.Show)
	main.Router.PUT("/api/nodes/:node", main.Nodes.Update)
	main.Router.GET("/api/nodes/:node/flash", main.Nodes.Firmware.Show)
	main.Router.POST("/api/nodes/:node/flash", main.Nodes.Firmware.Create)
	main.Router.GET("/api/nodes/:node/eeprom", main.Nodes.Firmware.Show)
	main.Router.POST("/api/nodes/:node/eeprom", main.Nodes.Firmware.Create)
	main.Router.GET("/api/interfaces", main.Interfaces.Index)
	main.Router.GET("/api/interfaces/:interf", main.Interfaces.Show)
	main.Router.PUT("/api/interfaces/:interf", main.Interfaces.Update)
	main.Router.GET("/api/jobs/:id", main.Jobs.Show)
	main.Router.GET("/api/jobs/:id/result", main.Jobs.Result)
	//main.Router.GET("/api/ports", main.Ports.Index)
	main.Router.ServeFiles("/static/*filepath", http.Dir("../static"))
	//main.Router.GET("/nodes", nodepage.Index)
	main.Router.GET("/", homepage.Index)

	main.Run()
	fmt.Println("Done")
}
