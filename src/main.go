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

// Main function of the service
func (p *program) run() {

	// Variables
	var url string
	var charge, chargelow int
	var cancharge, online, onbattery, wasonline, canconnect, looseconnect bool
	var err error

	canconnect = false
	looseconnect = false
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

	wasonline = false

	for {
		if server != "" {
			url = server + ":" + strconv.Itoa(port)
		} else {
			err := "No upsd server defined in config file. Please set server variable"
			hlog.Error(eventID, err)
			if debugmode {
				log.Println(err)
			}
			os.Exit(1)
		}

		if upsname == "" {
			err := "No ups defined in config file. Please set upsname variable"
			hlog.Error(eventID, err)
			if debugmode {
				log.Println(err)
			}
			os.Exit(1)
		}
		// Connect to the ups

		c, err := nutclient.Dial(url)
		if err != nil {
			if !canconnect {
				outtxt := "Cannot connect to upsd server : " + err.Error()
				hlog.Error(eventID, outtxt)
				if debugmode {
					log.Println(outtxt)
				}
				// only exit if first connect fail. Else retry (may be an upsd maintenance)
				os.Exit(1)
			} else {
				hlog.Warning(eventID, "Upsd server : "+err.Error()+" seems to be down ... Retrying later")
			}
		} else {
			canconnect = true

			if debugmode {
				fmt.Printf("Connected to %s", url)
			}

			// if tls set to 1, initiate tls session

			if usetls == 1 {
				tlsconfig := &tls.Config{
					InsecureSkipVerify: true,
					ServerName:         "localhost",
				}
				err = c.StartTLS(tlsconfig)
				if err != nil {
					outtxt := "Cannot start TLS session : " + err.Error()
					hlog.Error(eventID, outtxt)
					if debugmode {
						log.Println(outtxt)
					}
					os.Exit(1)
				}
				if debugmode {
					fmt.Print(" in SSL")
				}
			}
			if debugmode {
				fmt.Print("\n")
			}
			if login != "" && password != "" {
				// Authenticate against nut server
				err = c.Auth(login, password)
				if err != nil {
					outtxt := "Auth error  : " + err.Error()
					hlog.Error(eventID, outtxt)
					if debugmode {
						log.Println(outtxt)
					}
					os.Exit(1)
				}
			}

			// Select default ups
			err = c.Login(upsname)
			if err != nil {
				outtxt := "Cannot select ups " + upsname + " : " + err.Error()
				hlog.Error(eventID, outtxt)
				if debugmode {
					log.Println(outtxt)
				}
				os.Exit(1)
			}
			if debugmode {
				fmt.Printf("Using \"%s\" as current UPS :\n", upsname)
			}

			// if in debugmode, show ups informations
			if debugmode {
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

			cancharge = true

			charge, err = c.BatteryCharge()

			if err != nil {
				cancharge = false
			}

			chargelow, err = c.BatteryChargeLow()

			if err != nil {
				cancharge = false
			}

			online, err = c.IsOnline()
			if err != nil {
				hlog.Error(eventID, "Cannot get online status for ups "+upsname+" : "+err.Error())
				looseconnect = true
				if !wasonline {
					os.Exit(1)
				}
			}

			if online && !wasonline {
				hlog.Info(eventID, "Ups "+upsname+" is online")
				wasonline = true
			}

			if online && looseconnect {
				hlog.Info(eventID, "Ups "+upsname+" is back online")
				looseconnect = false
			}

			onbattery, err = c.IsOnBattery()
			if err != nil {
				hlog.Error(eventID, "Cannot get online status for ups "+upsname+" : "+err.Error())
				looseconnect = true
				if !wasonline {
					os.Exit(1)
				}
			}

			if onbattery && !wasonline {
				hlog.Warning(eventID, "Ups "+upsname+" is on battery but never seen it online - skipping shutdown (maybe voluntary restart ?) ")
			}

			if onbattery && looseconnect {
				hlog.Info(eventID, "Ups "+upsname+" is back on battery mode")
				looseconnect = false
			}

			if wasonline {
				if cancharge {
					if onbattery && !online {
						if charge <= chargelow {
							hlog.Warning(eventID, "Low Battery on "+upsname+". Shutting down system now !")
							c.Logout()
							c.Close()
							err = shutdown()
							if err != nil {
								hlog.Error(eventID, "Error when trying to shutdown system : "+err.Error())
								os.Exit(1)
							}
							os.Exit(1)
						} else {
							hlog.Warning(eventID, "Ups "+upsname+" is on battery - Current charge : "+strconv.Itoa(charge)+" , Low charge level defined at : "+strconv.Itoa(chargelow))
						}
					}
				} else {
					if onbattery && !online {
						hlog.Warning(eventID, "Low Battery on "+upsname+". Shutting down system now !")
						c.Logout()
						c.Close()
						err = shutdown()
						if err != nil {
							hlog.Error(eventID, "Error when trying to shutdown system : "+err.Error())
							os.Exit(1)
						}
						os.Exit(1)
					}
				}
			}

			c.Logout()
			c.Close()
		}

		// if debugmode then exit at first loop
		if debugmode {
			hlog.Info(eventID, serviceName+" - End of debug session ")
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
