<!--
#
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
-->

# Openwhisk Client Go
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0)
[![Build Status](https://travis-ci.org/apache/incubator-openwhisk-client-go.svg?branch=master)](https://travis-ci.org/apache/incubator-openwhisk-client-go)

This project `openwhisk-client-go` is a Go client library to access Openwhisk API.


### Prerequisites

You need to install the following package in order to run this Go client library:
- [Go](https://golang.org/doc/install)
- [govendor](https://github.com/kardianos/govendor)

Make sure you select the package that fits your local environment, and [set the GOPATH environment variable](https://github.com/golang/go/wiki/SettingGOPATH).


### Installation

After you download the source code either from the Github or the release page of OpenWhisk, you should have a directory named
_incubator-openwhisk-client-go_ to host all the source code. Please copy this root directory _incubator-openwhisk-client-go_
into the directory $GOPATH/src/github.com/apache.


### Test

Open a terminal, and run the following commands to run the unit tests:

```
$ cd $GOPATH/src/github.com/apache/incubator-openwhisk-client-go
$ govendor sync
$ go test -v ./... -tags=unit
```

You should see all the unit tests passed. If not, please [log an issue](https://github.com/apache/incubator-openwhisk-client-go/issues) for us.


### Configuration

This Go client library is used to access the OpenWhisk API, so please make sure you have an OpenWhisk service running somewhere
available for you to run this library.

We use a configuration file called _wskprop_ to specify all the parameters necessary for this Go client library to access the OpenWhisk
services. Make sure you create or edit the file _~/.wskprops_, and add the mandatory parameters APIHOST, APIVERSION, NAMESPACE and AUTH.

The parameter APIHOST is the OpenWhisk API hostname (for example, openwhisk.ng.bluemix.net, 172.17.0.1, and so on).
The parameter APIVERSION is the version of OpenWhisk API to be used to access the OpenWhisk resources.
The parameter NAMESPACE is the OpenWhisk namespace used to specify the OpenWhisk resources about to be accessed.
The parameter AUTH is the authentication key used to authenticate the incoming requests to the OpenWhisk services.

For more information regarding the REST API of OpenWhisk, please refer to [OpenWhisk REST API](https://github.com/apache/incubator-openwhisk/blob/master/docs/rest_api.md).


### Usage

```go
import "github.com/apache/incubator-openwhisk-client-go/whisk"
```

Construct a new whisk client, then use various services to access different parts of the whisk api.  For example to get the `hello` action:

```go
client, _ := whisk.NewClient(http.DefaultClient, nil)
action, resp, err := client.Actions.List("hello")
```

Some API methods have optional parameters that can be passed. For example, to list the first 30 actions, after the 30th action:
```go
client, _ := whisk.NewClient(http.DefaultClient, nil)

options := &whisk.ActionListOptions{
  Limit: 30,
  Skip: 30,
}

actions, resp, err := client.Actions.List(options)
```

By default, this Go client library is automatically configured by the configuration file _wskprop_. The parameters of APIHOST, APIVERSION,
NAMESPACE and AUTH will be used to access the OpenWhisk services.

In addition, it can also be configured by passing in a `*whisk.Config` object as the second argument to `whisk.New( ... )`.  For example:

```go
config := &whisk.Config{
  Host: "openwhisk.ng.bluemix.net",
  Version: "v1"
  Namespace: "_",
  AuthKey: "aaaaa-bbbbb-ccccc-ddddd-eeeee"
}
client, err := whisk.Newclient(http.DefaultClient, config)
```


### Example

You need to have an OpenWhisk service accessible, to run the following example.

Please be advised that all the Go files you are about to create should be under the directory of $GOPATH or its subdirectories.
For example, create the Go file named _openwhisk_client_go.go_ under a directory called $GOPATH/src/example to try the following code.

```go
import (
  "net/http"
  "net/url"

  "github.com/apache/incubator-openwhisk-client-go/whisk"
)

func main() {
  client, err := whisk.NewClient(http.DefaultClient, nil)
  if err != nil {
    fmt.Println(err)
    os.Exit(-1)
  }

  options := &whisk.ActionListOptions{
    Limit: 30,
    Skip: 30,
  }

  actions, resp, err := client.Actions.List(options)
  if err != nil {
    fmt.Println(err)
    os.Exit(-1)
  }

  fmt.Println("Returned with status: ", resp.Status)
  fmt.Println("Returned actions: \n%+v", actions)

}
```

Then build it with the go tool:

```
$ cd $GOPATH/src/example
$ go build
```

The command above will build an executable named client in the directory alongside your source code. Execute it to see the the result:


$ ./openwhisk_client_go

If the openWhisk service is available and your configuration is correct, you should receive the status and the actions with
the above example.


# Disclaimer
Apache OpenWhisk Client Go is an effort undergoing incubation at The Apache Software Foundation (ASF), sponsored by the Apache Incubator. Incubation is required of all newly accepted projects until a further review indicates that the infrastructure, communications, and decision making process have stabilized in a manner consistent with other successful ASF projects. While incubation status is not necessarily a reflection of the completeness or stability of the code, it does indicate that the project has yet to be fully endorsed by the ASF.
