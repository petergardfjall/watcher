`watcher` is a simple monitoring tool that checks the health of a number of
remote servers. It does this by regularly executing a set of *pingers*, each
performing a check against a remote server using a certain *ping protocol*.
Currently, pingers based on SSH and HTTP(S) are supported.

Should a ping check fail to produce an expected result, `watcher` will send
out an alert email to any configured recipients. Should the remote endpoint
yet again become availble, a new alert will be sent to notify that the endpoint
has recovered. Should the ping check continue fail for a sufficiently long 
time, reminder alerts will be sent (after a configurable time interval).

Besides sending out email alerts, the `watcher` also publishes a simple REST
API, through which the status of its configured endpoints can be viewed.



## Build
The `watcher` binary can be built as follows:
    
    # get dependencies
    go get -u 
    # compile
    go build



## Configure

`watcher` expects a JSON-formatted configuration file, which describes the
ping checks to perform, at what interval, and how to alert (if at all) when
a monitored server's state changes.

The configuration file has the following structure:

```
{
    "defaultSchedule": {
        "interval": "1m",
        "retries": {
            "attempts": 3,
            "delay": "10s",
            "exponentialBackoff": false
        }
    },
    "pingers": [
        {
            "name": "pinger-name",
            "description": "pinger description",
            "type": "<protocol>",
            "check": {
			   <protocol-specific>
            },
            "schedule": {
                "interval": "90s",
                "retries": { "attempts": 3, "delay": "5s", "exponentialBackoff": false}
            }
        },	
	    ...
    ],
    "alerter": {
        "reminderDelay": "12h",
        "email": {
            "smtpHost": "some.smtp.host",
            "smtpPort": 587,
            "auth": { "username": "foo", "password": "bar" },
            "from": "noreply@watcher.org",
            "to": ["admin@company.com"]
        }
    }
}
```

- `defaultSchedule` (optional): specifies a default schedule to 
  use for running pingers
  and also the default number of attempts to make on each ping. The default
  schedule can be overridden for each pinger, since a separate schedule can be
  specified per-pinger. Default is `interval`: `10m`, 
  `attempts`: `3`, `delay`: `3s`, `exponentialBackoff`: `false`.
    - `interval`: The default duration between two successive ping checks. This
	  is specified as a 
	  [golang duration](https://golang.org/pkg/time/#ParseDuration). For
	  example, `1h` (1 hour), `5m30s` (5 minutes and 30 seconds).
	- `retries`: The default retry behavior of pingers.
	    - `attempts`: The number of attempts to try before deeming a ping
		  check a failure.
		- `delay`: Delay between each retry. Also given as a 
          [golang duration](https://golang.org/pkg/time/#ParseDuration). For
          example, `2m` (2 minutes).
		- `exponentialBackoff`: If `true`, double the delay for each new 
		  retry attempt.
- `pingers`: The set of pingers to run.
    - Each pinger is an object with the following fields:
        - `name`: The name of the pinger. Can only contain alphanumeric 
		  characters and `-`, `.`, and `_`.
		- `description` (optional): A short description of the pinger and 
		  its purpose.
		- `type`: The type (protocol) of the pinger. One of `ssh` and `http`.
		- `check`: Protocol-specific details on how to perform each "ping".
		   See below.
		- `schedule` (optional): The schedule to use for this pinger. If no
		  schedule is given, the `defaultSchedule` is used.		
- `alerter` (optional): How to notify interested parties of state changes in
  monitored endpoints.
    - `reminderDelay`: The duration to wait before sending out a reminder alert
	  for an endpoint that keeps failing to respond properly on ping attempts.
	  This is specified as a 
	  [golang duration](https://golang.org/pkg/time/#ParseDuration). For
	  example, `1h` (1 hour), `90m` (90 minutes).
	- `email`: Configures an email alerter that will send alerts to a set of
	  email recipients.
	    - `smtpHost`: The SMTP server to send mails through.
		- `smtpPort`: The port of the SMTP server (typically `587`, `25` 
		  or `465`)
        - `auth` (optional): Authentication credentials (a `username` 
		  and `password`).
        - `from`: The `From:` address to set on sent alerts.
        - `to`: a list of email addresses to send alerts to (`To:`).


A `http` pinger, which tries to contact a URL (via a `GET` request) and 
expects a certain HTTP status code in the response, is configured as follows:

```
{
    "name": "<name>",
    "type": "http",
    "check": {
        "url": "https://...",
		"verifyCert": false,
		"basicAuth": {"username": "foo", "password": "bar"},
        "expect": {
            "statusCode": 200
         },
         "timeout": "10s"
    }
}	
```


The `check` is the only part specific to the `http` pinger. Its fields
carry the following semantics:

- `url`: The URL to try and contact.
- `verifyCert`: If `true`, the server's certificate will be verified. If 
  `false` no such verification is made (similar to `curl`'s `--insecure` flag).
- `basicAuth` (optional): Specifies username and password to use.
- `expect`: The expected response for the pinger to deem a ping attempt a 
  success.
    - `statusCode`: The HTTP status code that the endpoint needs to respond 
	  with.
- `timeout` (optional): The connection timeout to use. Default: `30s`.



A `ssh` pinger, which executes a shell command/script against a remote server 
and expects a certain exit code, is configured as shown below:

```
{
    "name": "<name>",
    "type": "ssh",
    "check": {
       "host": "some.host",
       "port": 22,
       "auth": { "username": "foo", "agent": true },
       "command": "service docker status | grep running",
       "timeout": "10s",
       "expect": {
           "exitCode": 0
        }
     }
}
```

The `check` is the only part specific to the `ssh` pinger. Its fields
carry the following semantics:

- `host`: The remote SSH server to monitor.
- `port`: The port number of the SSH server (typically `22`).
- `auth`: 
    - `username`: The username to log in with.  	  
    An authentication mechanism also needs to be specified. 
	Must be *at least* one of the following: 
        - `agent`: `true` if the [agent connection authentication method](https://en.wikipedia.org/wiki/Ssh-agent) is to be used.
        - `password`: Specifies a password to use.
        - `key`: Specifies the path to a private key to use with the public 
		   key authentication method.
- The `check` must also specify a shell command/script to execute. It is either 
  given directly as a `command` or as a file path via `commandFile`.
- `timeout` (optional): The connection timeout to use. Default: `30s`.
- `expect`: The expected response for the pinger to deem a ping attempt a 
  success.
    - `exitCode`: The exit code that the script must produce for the ping to be
	  successful.

Sample configurations are given under `etc/`.



# Run

To run `watcher` according to a given configuration just run:

    ./watcher --certfile cert.pem --keyfile key.pem config.json
	
This will start a watcher publishing a HTTPS server on port `8443` (see below
for a description of its REST API). A self-signed certificate and key can be 
generated using the `etc/cert/generate-server-cert.sh` script.

For a complete list of command-line options, run `./watcher --help`.



## REST API

`watcher` publishes the following endpoints:

### List all configured pingers:
``` 
$ curl --insecure https://localhost:8443/pingers/
[
    "https://localhost:8443/pingers/google.com",
    "https://localhost:8443/pingers/jenkins",
]
```


### Get status of a given pinger
``` 
$ curl --insecure https://localhost:8443/pingers/google.com
{
    "LatestResult": {
        "Status": 2,
        "Error": {}
    },
    "Consecutive": 2,
    "LatestOK": null,
    "LatestNOK": "2016-05-26T09:38:57.686217751Z"
}
```
Status `0` means `Unknown`, `1` means `OK`, and 2 means `NOK`.



### Get latest output of a given pinger
``` 
$ curl --insecure https://localhost:8443/pingers/google.com/output
...
```

