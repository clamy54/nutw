/*
Nutw is a NUT client for Windows.
Copyright (C) 2022 Cyril LAMY.

This program is free software; you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation; either version 2 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

For the complete terms of the GNU General Public License, please see this URL:
http://www.gnu.org/licenses/gpl-2.0.html
*/
package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"github.com/clamy54/nutclient"
	"github.com/kardianos/service"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"gopkg.in/ini.v1"
)

const eventID uint32 = 177
const serviceName = "Nutw"
const displayName = "Nutd windows client"
const desc = "Shutdown system properly when power supply errors occur"

type program struct{}

var hlog *eventlog.Log
var triggersvc, debugmode bool

// Get ini file path (must be in the same directory as the .exe file)
func getInifilePath() (string, error) {
	var s string
	var err error
	s, err = os.Executable()
	if err != nil {
		return "", err
	}
	ext := filepath.Ext(s)
	outfile := s[0:len(s)-len(ext)] + ".ini"
	return outfile, nil
}

// Set ShutdownPrivilege token
func setSeShutdownNamePrivilege() error {
	// Get current process (the one I wanna change)
	handle := windows.CurrentProcess()
	defer windows.CloseHandle(handle)

	// Get the current process token
	var token windows.Token
	err := windows.OpenProcessToken(handle, windows.TOKEN_ADJUST_PRIVILEGES, &token)
	if err != nil {
		hlog.Error(eventID, "Error when calling OpenProcessToken function : "+err.Error())
	}

	// Check the LUID
	var luid windows.LUID
	sePrivName, err := windows.UTF16FromString("SeShutdownPrivilege")
	if err != nil {
		hlog.Error(eventID, "Error when calling UTF16FromString : "+err.Error())
	}
	err = windows.LookupPrivilegeValue(nil, &sePrivName[0], &luid)
	if err != nil {
		hlog.Error(eventID, "Error when lookup for SeShutdownPrivilege privilege : "+err.Error())
	}

	// Modify the token
	var tokenPriviledges windows.Tokenprivileges
	tokenPriviledges.PrivilegeCount = 1
	tokenPriviledges.Privileges[0].Luid = luid
	tokenPriviledges.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED

	// Adjust token privs
	tokPrivLen := uint32(unsafe.Sizeof(tokenPriviledges))
	//fmt.Printf("Length is %d\n", tokPrivLen)
	err = windows.AdjustTokenPrivileges(token, false, &tokenPriviledges, tokPrivLen, nil, nil)
	if err != nil {
		hlog.Error(eventID, "Error when calling AdjustTokenPrivileges function: "+err.Error())
		return err
	}
	//fmt.Println("[+] Debug Priviledge granted")
	return nil
}

// Shutdown OS and add an error entry in Windows event logger
func shutdown() error {

	const ewxForceIfHung = 0x00000010
	const ewxPowerOff = 0x00000008
	const shutdownReasonPower = 0x00060000
	const shutdownReasonMinorEnvironment = 0x0000000c

	if !debugmode {
		err := setSeShutdownNamePrivilege()

		if err != nil {
			hlog.Error(eventID, "Error when setting SeShutdownPrivilege privilege: "+err.Error())
		}

		user32 := syscall.MustLoadDLL("user32")
		defer user32.Release()

		ExitWindowsEx := user32.MustFindProc("ExitWindowsEx")
		result, _, err := ExitWindowsEx.Call(ewxPowerOff|ewxForceIfHung, shutdownReasonPower|shutdownReasonMinorEnvironment)
		if result != 1 {
			return err
		}
	}
	return nil
}

// upsprobe holds the settings needed to reach the ups, and remembers what
// has been seen during the previous probes.
type upsprobe struct {
	url      string
	usetls   bool
	login    string
	password string
	upsname  string

	reached      bool // the ups has been probed successfully at least once
	wasonline    bool // the ups has been seen online at least once
	looseconnect bool // the previous probe failed
}

// configError reports a failure the upsd server itself complained about,
// such as a bad password or an unknown ups name. Retrying will not help
// until the ini file is fixed.
type configError struct {
	err error
}

func (e *configError) Error() string {
	return e.err.Error()
}

// probe runs a single probe of the ups. It returns the reason why the
// system has to be shut down, or an empty string when there is nothing to
// do. A probe failure is returned as an error and is never fatal by
// itself : the ups may just be rebooting, or a switch may be reloading
// its firmware, which is no reason to stop the service nor to shutdown
// the machine.
func (u *upsprobe) probe() (string, error) {

	// Connect to the ups
	c, err := nutclient.Dial(u.url)
	if err != nil {
		return "", fmt.Errorf("cannot connect to upsd server : %w", err)
	}

	defer c.Close()

	if debugmode {
		fmt.Printf("Connected to %s", u.url)
	}

	// if tls set to 1, initiate tls session
	if u.usetls {
		tlsconfig := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "localhost",
		}

		err = c.StartTLS(tlsconfig)
		if err != nil {
			return "", fmt.Errorf("cannot start TLS session : %w", err)
		}

		if debugmode {
			fmt.Print(" in SSL")
		}
	}

	if debugmode {
		fmt.Print("\n")
	}

	if u.login != "" && u.password != "" {
		// Authenticate against nut server
		err = c.Auth(u.login, u.password)
		if err != nil {
			return "", &configError{fmt.Errorf("auth error : %w", err)}
		}
	}

	// Select default ups
	err = c.Login(u.upsname)
	if err != nil {
		return "", &configError{fmt.Errorf("cannot select ups %s : %w", u.upsname, err)}
	}

	defer c.Logout()

	if debugmode {
		fmt.Printf("Using \"%s\" as current UPS :\n", u.upsname)

		// show ups informations
		params, err := c.GetUpsVars()
		if err == nil {
			for _, value := range params {
				dataparam, _ := c.GetData(value)
				fmt.Printf("%s : \"%v\" \n", value, dataparam)
			}
		} else {
			log.Println("Cannot fetch ups vars")
		}
	}

	// Read the status before anything else : a shutdown must never be
	// decided from the values read during a previous probe.
	online, err := c.IsOnline()
	if err != nil {
		return "", fmt.Errorf("cannot get online status for ups %s : %w", u.upsname, err)
	}

	onbattery, err := c.IsOnBattery()
	if err != nil {
		return "", fmt.Errorf("cannot get battery status for ups %s : %w", u.upsname, err)
	}

	// The LB flag is the low battery warning raised by the ups itself, it
	// does not depend on the battery level probes.
	canlowbattery := true

	lowbattery, err := c.IsLowBattery()

	if err != nil {
		canlowbattery = false
	}

	cancharge := true

	charge, err := c.BatteryCharge()

	if err != nil {
		cancharge = false
	}

	chargelow, err := c.BatteryChargeLow()

	if err != nil {
		cancharge = false
	}

	u.reached = true

	if online && !u.wasonline {
		hlog.Info(eventID, "Ups "+u.upsname+" is online")
		u.wasonline = true
	}

	if u.looseconnect {
		if online {
			hlog.Info(eventID, "Ups "+u.upsname+" is back online")
		}

		if onbattery {
			hlog.Info(eventID, "Ups "+u.upsname+" is back on battery mode")
		}

		u.looseconnect = false
	}

	if onbattery && !u.wasonline {
		hlog.Warning(eventID, "Ups "+u.upsname+" is on battery but never seen it online - skipping shutdown (maybe voluntary restart ?) ")
	}

	if !u.wasonline || !onbattery || online {
		return "", nil
	}

	switch {
	case canlowbattery && lowbattery:
		// The ups says it is running out of power, whatever the charge
		// level says.
		return "Ups " + u.upsname + " reports a low battery", nil

	case cancharge && charge <= chargelow:
		return "Low Battery on " + u.upsname + " - Current charge : " + strconv.Itoa(charge) + " , Low charge level defined at : " + strconv.Itoa(chargelow), nil

	case !cancharge && !canlowbattery:
		// Neither the charge level nor the LB flag can be read, so there
		// is no way to tell how much runtime is left : shutdown while we
		// still can.
		return "Ups " + u.upsname + " is on battery and reports neither its charge nor a low battery flag", nil

	case cancharge:
		hlog.Warning(eventID, "Ups "+u.upsname+" is on battery - Current charge : "+strconv.Itoa(charge)+" , Low charge level defined at : "+strconv.Itoa(chargelow))

	default:
		hlog.Warning(eventID, "Ups "+u.upsname+" is on battery - Waiting for the low battery flag")
	}

	return "", nil
}

// Main function of the service
func (p *program) run() {

	// Variables
	var err error

	// Setup logger (in run manually before service installed)

	_ = eventlog.InstallAsEventCreate(serviceName, eventlog.Info|eventlog.Warning|eventlog.Error)

	//Open a handler for logs
	hlog, err = eventlog.Open(serviceName)
	if err != nil {
		log.Println(err)
	}

	defer hlog.Close()

	if debugmode {
		hlog.Info(eventID, serviceName+" - Starting debug session ")
	}

	// Read ini file
	inifile, err := getInifilePath()
	if err != nil {
		hlog.Error(eventID, "Cannot get ini file path")
		os.Exit(1)
	}

	cfg, err := ini.Load(inifile)
	if err != nil {
		hlog.Error(eventID, "Failed to read config file")
		os.Exit(1)
	}
	server := cfg.Section("nutd").Key("server").MustString("")
	port := cfg.Section("nutd").Key("port").MustInt(3493)
	usetls := cfg.Section("nutd").Key("usetls").MustInt(0)
	login := cfg.Section("nutd").Key("login").MustString("")
	password := cfg.Section("nutd").Key("password").MustString("")
	upsname := cfg.Section("nutd").Key("upsname").MustString("")
	interval := cfg.Section("nutd").Key("interval").MustInt(60)

	if interval < 10 {
		interval = 10
	}

	if server == "" {
		outtxt := "No upsd server defined in config file. Please set server variable"
		hlog.Error(eventID, outtxt)
		if debugmode {
			log.Println(outtxt)
		}
		os.Exit(1)
	}

	if upsname == "" {
		outtxt := "No ups defined in config file. Please set upsname variable"
		hlog.Error(eventID, outtxt)
		if debugmode {
			log.Println(outtxt)
		}
		os.Exit(1)
	}

	ups := &upsprobe{
		url:      server + ":" + strconv.Itoa(port),
		usetls:   usetls == 1,
		login:    login,
		password: password,
		upsname:  upsname,
	}

	for {
		reason, err := ups.probe()

		if err != nil {
			var cfgerr *configError

			if errors.As(err, &cfgerr) && !ups.reached {
				// Upsd rejected us on the very first probe, the ini file
				// is most likely wrong and probing again will not help.
				// Once the ups has been reached, the same answer is
				// treated as a temporary failure : upsd also rejects a
				// LOGIN while its driver is restarting.
				hlog.Error(eventID, err.Error())
				if debugmode {
					log.Println(err.Error())
				}
				os.Exit(1)
			}

			// The ups is unreachable : keep probing, it may be rebooting,
			// under maintenance, or the network may not be up yet. Only
			// the first failure is logged, so a long outage does not fill
			// up the event log.
			if !ups.looseconnect {
				if ups.reached {
					hlog.Warning(eventID, "Upsd server : "+err.Error()+" seems to be down ... Retrying later")
				} else {
					hlog.Error(eventID, "Cannot probe upsd server : "+err.Error()+" - Retrying later")
				}
				ups.looseconnect = true
			}

			if debugmode {
				log.Println(err.Error())
			}
		}

		if reason != "" {
			hlog.Warning(eventID, reason+". Shutting down system now !")
			err = shutdown()
			if err != nil {
				hlog.Error(eventID, "Error when trying to shutdown system : "+err.Error())
			}
			os.Exit(1)
		}

		// if debugmode then exit at first loop
		if debugmode {
			hlog.Info(eventID, serviceName+" - End of debug session ")
			if err != nil {
				os.Exit(1)
			}
			os.Exit(0)
		}

		// exit loop it triggersvc set to true, else wait and rerun probe
		if !triggersvc {
			time.Sleep(time.Duration(interval) * time.Second)
		} else {
			hlog.Info(eventID, "Shutting down service "+serviceName)
			os.Exit(0)
		}

	}
}

// Manage service cyclelife

func (p *program) Start(s service.Service) error {
	// Should be non-blocking, so run async using goroutine
	triggersvc = false
	go p.run()
	return nil
}

func (p *program) Stop(s service.Service) error {
	// Should be non-blocking
	triggersvc = true
	return nil
}

// Entry point of main program
func main() {

	var mode string

	debugmode = false
	inService, err := svc.IsWindowsService()

	if err != nil {
		log.Fatalf("failed to determine if we are running in service: %v", err)
	}

	svcConfig := &service.Config{
		Name:        serviceName,
		DisplayName: displayName,
		Description: desc,
	}

	// if runned as a service
	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	if inService {
		err = s.Run()
		if err != nil {
			log.Fatal(err)
		}
	} else {
		// else if runned from cli
		flag.StringVar(&mode, "mode", "", "install/uninstall/debug/start/stop/restart")
		flag.Parse()

		// manage service lifecycle
		if mode == "install" {
			err = s.Install()
			if err != nil {
				log.Fatal(err)
			}
			err = s.Start()
			if err != nil {
				log.Fatal(err)
			}
		}

		if mode == "start" {
			err = s.Start()
			if err != nil {
				log.Fatal(err)
			}
		}

		if mode == "stop" {
			err = s.Stop()
			if err != nil {
				log.Fatal(err)
			}
		}

		if mode == "restart" {
			err = s.Restart()
			if err != nil {
				log.Fatal(err)
			}
		}

		if mode == "uninstall" {
			_ = s.Stop()

			err = s.Uninstall()
			if err != nil {
				log.Fatal(err)
			}
		}

		if mode == "debug" {
			debugmode = true
			triggersvc = true
			err = s.Run()
			if err != nil {
				log.Fatal(err)
			}
		}

		// if no args specified
		if mode == "" {
			fmt.Fprintf(os.Stderr,
				"%s \n(c)2022-2023 Cyril LAMY\n\n"+
					"usage: %s --mode=[install,uninstall,debug,start,stop]\n"+
					"       install: install and start service (automatic startup at boot enabled) \n"+
					"       uninstall: stop and uninstall service\n"+
					"       debug: run once, no shutdown\n"+
					"       start: start service (must be installed before)\n"+
					"       stop: stop service (must be installed before)\n"+
					"       restart: restart service (must be installed and running before)\n"+
					"\n"+
					"Logs are written to the Applications Event Log, which can be viewed through the Event Viewer in Administrative Tools (filter by ID %v ).\n",
				displayName, os.Args[0], eventID)
			os.Exit(2)
		}
	}
}
