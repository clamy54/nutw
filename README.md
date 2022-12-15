Nutw
====

## About

Nutw is a NUT client for Windows.
Nutw is running as a service and probes a NUT server at regular intervals.

If ups supports battery levels probes, then Nutw shutdown the local OS if
current battery level is below low-battery level.

If ups doesn't battery levels probes, then Nutw shutdown the local OS if
ups is going on battery.

## Installation

Just run nutw-install.exe with administrator privileges and configure installation folder.

## Configuration

Open nutw.ini (installed in the same directory as nutw.exe during installation) and configure the
nutd section.

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

Nutw is running as a windows service. After configuration and testings are done, you can setup the service by opening a terminal in Administrator mode,
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

## Credits

Changelog :

v1.1 - Debug mode is now more verbose on default console
v1.0 - Initial release

## Credits

Nutw use [Kardianos Service module](https://github.com/kardianos/service).

