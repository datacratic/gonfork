# gonfork #

HTTP live traffic duplication.

When placed in an HTTP stream, nfork duplicates each received requests to
multiple configured servers and forwards the response of the configured active
endpoint back upstream. All other responses are dropped.

Useful for staging new changes on a shadow stack without affecting the
production stack.

## Installation ##

You can download the code via the usual go utilities:

```
go get github.com/datacratic/gonfork/nfork
go get github.com/datacratic/gonfork/nforkd
```

To build the code and run the test suite along with several static analysis
tools, use the provided Makefile:

```
make test
```

Note that the usual go utilities will work just fine but we require that all
commits pass the full suite of tests and static analysis tools.


## Configuration ##

An nfork inbound endpoint is configured using JSON. Here's a simple example:

```javascript
    {
        "name": "foo",
        "listen": ":9080",
        "out": {
            "prod": "localhost:8080",
            "staging": "localhost:8081",
            "logging": "localhost:8082"
        },
        "active": "prod",
        "timeout": "100ms",
        "timeoutCode": 500,
        "idleConn": 64
    }

```

| Key | Description |
| --- | --- |
| `name` | Name used to refer to the inbound endpoint |
| `listen` | Where to listen for the incoming HTTP stream |
| `out` | Set of named outbound backends where the HTTP stream will be duplicated to |
| `active` | Name of the outbound backend whose response will be forwarded |
| `timeout` | Requests will expire after this amount of time (optional) |
| `timeoutCode` | Use this HTTP status code in the event of a time out (optional) |
| `idleConn` | Size of the idle connection pool (optional) |

The initial configuration for the nfork daemon `nforkd` is passed using the
command line argument `--config` which points to a file containing an array of
inbound endpoint (eg. [nfork.json](nfork.json)).

Once started, `nforkd` provides a REST interface.

| Path | Method | Description |
| --- | --- | --- |
| `/v1/nfork` | `GET` | Returns all inbound endpoints |
| `/v1/nfork` | `POST` | Add an inbound endpoint |
| `/v1/nfork/stats` | `GET` | Returns the stats for all inbound endpoints |
| `/v1/nfork/:inbound` | `GET` | Returns the given inbound endpoint |
| `/v1/nfork/:inbound` | `DELETE` | Removes the given inbound endpoint |
| `/v1/nfork/:inbound/stats` | `GET` | Returns the stats for the given inbound endpoint |
| `/v1/nfork/:inbound/:outbound` | `PUT` | Add an outbound endpoint to the given inbound endpoint |
| `/v1/nfork/:inbound/:outbound` | `DELETE` | Removes the given outbound endpoint |
| `/v1/nfork/:inbound/:outbound/stats` | `GET` | Returns the stats of the given outbound endpoint |
| `/debug/klog` |  | See the [klog](http://github.com/datacratic/goklog) documentation |
| `/debug/pprof` | `GET` | See the [pprof](https://godoc.org/net/http/pprof) documentation |

## License ##

The source code is available under the Apache License. See the LICENSE file for
more details.
