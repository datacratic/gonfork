# gonfork #

Network traffic duplication where only the response of one backend is kept.

## Installation ##

You can download the code via the usual go utilities:

```
go get github.com/datacratic/gonfork
```

To build the code and run the test suite along with several static analysis
tools, use the provided Makefile:

```
make test
```

Note that the usual go utilities will work just fine but we require that all
commits pass the full suite of tests and static analysis tools.


## License ##

The source code is available under the Apache License. See the LICENSE file for
more details.
