Nutw
====

## About

Nutw is a NUT client for Windows.
Nutw is running as a service and probes a NUT server at regular intervals.

If ups raises its low battery flag (LB), then Nutw shutdown the local OS,
whatever the battery level is.

If ups supports battery levels probes, then Nutw shutdown the local OS if
current battery level is below low-battery level.

If ups supports neither the low battery flag nor battery levels probes,
then Nutw shutdown the local OS if ups is going on battery.

## Installation

Download `nutw-install.exe` from the [releases page](https://github.com/clamy54/nutw/releases)
and run it with administrator privileges. Administrator privileges are required : Nutw registers
itself as a service and needs the right to shutdown the machine.

The installer asks for the installation folder, then for the settings of the nut server. It writes
them to nutw.ini and, if the corresponding task is checked on the last page, installs and starts
the Nutw service, so there is nothing left to do afterwards.

Running the installer again over an existing installation keeps the current settings : they are
loaded in the wizard, the service is stopped before the files are replaced, then installed again.

## Configuration

The settings are the ones asked by the installer. They can be changed at any time by editing
nutw.ini (installed in the same directory as nutw.exe) and restarting the service with
`nutw --mode=restart`.

| Keyword:               | Description:                                               | 
| ---------------------- | -----------------------------------------------------------|
| `server`               | `hostname or ip adress of the upsd daemon server`          |
| `port`                 | `tcp port used by the remote upsd daemon`                  |
| `usetls`               | `if set to 1 then initiate tls session`                    |
| `login`                | `login to use for authenticating against upsd daemon`      |
| `password`             | `password to use for authenticating against upsd daemon`   |
| `upsname`              | `ups device name served by upsd daemon`                    |
| `interval`             | `amount of time (in seconds) between two probes`           |


## Testing the configuration

Open a terminal, change directory to Nutw installation directory and run :

```
nutw --mode=debug
```

Nutw logs can be checked with the help of 'Windows Event Viewer'. Just expand Applications logs and filter by ID 177.

## Setting up the service

Nutw is running as a windows service. The installer can set it up, but if the task was left
unchecked, you can setup the service by opening a terminal in Administrator mode,
changing current directory to Nutw installation directory and run :

```
nutw --mode=install
```

Nutw logs can be checked with the help of 'Windows Event Viewer' (see Testing section).

## Nutw parameters

Nutw support the following mode  :

| Keyword:               | Description:                                               | 
| ---------------------- | -----------------------------------------------------------|
| `install`              | `install and run the service`                              |
| `uninstall`            | `stop and uninstall the service`                           |
| `start `               | `start the service (must be installed before)`             |
| `stop`                 | `stop the service`                                         |
| `restart`              | `restart the service (must be running)`                    |
| `debug`                | `test Nutw configuration ( No OS shutdown in this mode )`  |


Usage :
```
nutw --mode=<Keyword>
```

Some modes requires Administrator privileges.

## Building

The installer published on the releases page is built by GitHub Actions on every tag. To build it
locally, [Inno Setup 6](https://jrsoftware.org/isinfo.php) is needed :

```
go build -trimpath -ldflags "-s -w" -o build\nutw.exe .\src
"C:\Program Files (x86)\Inno Setup 6\ISCC.exe" /DAppVersion=1.4 installer\nutw.iss
```

The installer is written to dist\nutw-install.exe .

## Changelog

v2.0 :

* The service does not stop anymore when the ups becomes unreachable. Any failed probe is now
  logged and retried at the next interval, whatever the reason : upsd maintenance, ups firmware
  update, switch reload or network outage
* Losing contact with the ups cannot shutdown the machine anymore. The previous version kept the
  values read during the previous probe when a read failed, so an outage happening right after
  the ups went on battery could shutdown a machine that was still powered
* The low battery flag (LB) raised by the ups is now honoured, whatever the reported charge is.
  It takes the ageing of the battery into account, which a fixed percentage threshold ignores
* Battery levels are read again on the ups reporting them as floating point values, such as
  "100.0". They were previously rejected, which silently disabled the low charge threshold and
  shut the machine down at the first power outage
* A service stop request is honoured right away instead of being noticed up to interval seconds
  later, which made the service manager give up
* The installer now asks for the settings and writes nutw.ini, and can install and start the
  service. It also keeps the settings of a previous installation
* The installer is published on the releases page and built by GitHub Actions

v1.3 - Do not exit the service immediately when the ups goes offline, log the connection losses

v1.2 - Build using nutclient v1.0.2, do not overwrite an existing ini file at install

v1.1 - Debug mode is now more verbose on default console

v1.0 - Initial release

## Credits

Nutw use [Kardianos Service module](https://github.com/kardianos/service).

