# Logstash hook for logrus <img src="http://i.imgur.com/hTeVwmJ.png" width="40" height="40" alt=":walrus:" class="emoji" title=":walrus:" />
[![Build Status](https://travis-ci.org/bshuster-repo/logrus-logstash-hook.svg?branch=master)](https://travis-ci.org/bshuster-repo/logrus-logstash-hook)
[![Go Report Status](https://goreportcard.com/badge/github.com/nekomeowww/logrus-logstash-hook)](https://goreportcard.com/report/github.com/nekomeowww/logrus-logstash-hook)

Use this hook to send the logs to [Logstash](https://www.elastic.co/products/logstash).

Improved with better callframe override abilities and better support for elasticsearch index patterns by merging additional fields into `key=val` format and store them into `fields` field in elasticsearch document.

## Usage

### Logstash configuration

#### Send to Elasticsearch Cluster

```conf
input {
    tcp {
        port => 8911 # Select a free and opening port
        codec => json
        tcp_keep_alive => true # optional: keep the connection alive
    }
}

output {
    elasticsearch {
        hosts => ["https://localhost:9200"]
        ssl => true
        cacert => "" # optional: path to elasticsearch host CA certificates, such as /etc/logstash/certs/ca.crt
        api_key => "" # Kibana generated API key, if you dont have API key configurated, use username and password instead
        index => "" # optional: Specify the index you wish to send to
    }
}
```

#### Directly print in console

```conf
input {
    tcp {
        port => 8911 # Select a free and opening port
        codec => json
        tcp_keep_alive => true # optional: keep the connection alive
    }
}

output {
    stdout {
        codec => rubydebug
    }
}
```

### Golang code references

#### General usage

```go
package main

import (
        "net"

        "github.com/sirupsen/logrus"
        logrustash "github.com/nekomeowww/logrus-logstash-hook"
)

func main() {
        // new logrus instance
        logger := logrus.New()

        // these fields will be at the top-level fields of documents
        predefinedFields := logrus.Fields{"type": "myappName"}

        // create a hook to send logs to Logstash
        hook, err := logrustash.New("tcp", "logstash.mycompany.net:8911", logrustash.DefaultFormatter(predefinedFields))
        if err != nil {
                log.Fatal(err)
        }

        // add this hook to the logger
        logger.Hooks.Add(hook)

        // this package will merge the non-pre-defined fields into key=val format,
        // and store them into fields field of the document for better elasticsearch compatibility,
        // may also reduce the time to deal with the elasticsearch index template
        laterAddedFields := logrus.Fields{"exampleFields", "fields"}

        // then just use as normal logrus
        logger.WithFields(laterAddedFields).Info("Hello World!")
}
```

This is how it will look like:

```ruby
{
    "@timestamp" => "2016-02-29T16:57:23.000Z",
      "@version" => "1",
         "level" => "info",
       "message" => "Hello World!",
        "fields" => "exampleFields=fields", # merged fields
          "host" => "172.17.0.1",
          "port" => 45199,
          "type" => "myappName"
}
```

#### With caller information

```go
package main

import (
        "net"
        "runtime"

        "github.com/sirupsen/logrus"
        logrustash "github.com/nekomeowww/logrus-logstash-hook"


)

var Log *logrus.Logger

func setCaller(entry *logrus.Entry) {
        // get the caller context
        pc, file, line, _ := runtime.Caller(1)

        // transform pc pointer to *runtime.Func
        funcDetail := runtime.FuncForPC(pc)
        var funcName string
        if funcDetail != nil {
                // get the function name
                funcName = funcDetail.Name()
        }

        // set the caller context into entry.Context
        entry.Context = context.WithValue(context.Background(), logrustash.ContextKeyRuntimeCaller, &runtime.Frame{
                File:     file,
                Line:     line,
                Function: funcName,
        })
}

func Info(args ...interface{}) {
        entry := logrus.NewEntry(Log)
        setCaller(entry, 1)
        entry.Info(args...)
}

func main() {
        // new logrus instance
        Log = logrus.New()
        // set ReportCaller to true to get the callframe
        Log.SetReportCaller(true)

        // these fields will be at the top-level fields of documents
        predefinedFields := logrus.Fields{"type": "myappName"}

        // create a hook to send logs to Logstash
        hook, err := logrustash.New("tcp", "logstash.mycompany.net:8911", logrustash.DefaultFormatter(predefinedFields))
        if err != nil {
                log.Fatal(err)
        }

        // add this hook to the logger
        Log.Hooks.Add(hook)

        // then just use as normal logrus
        Info("Hello World!")
}
```

```ruby
{
    "@timestamp" => 2022-07-15T09:22:34Z,
      "@version" => "1",
          "file" => "<path/to/code>:1",
      "function" => "main",
         "level" => "info",
       "message" => "Hello World",
          "type" => "log"
}
```

## Original Creator

[Boaz Shuster](https://github.com/bshuster-repo)

## Maintainers

Name         | Github         |
------------ | -------------- |
Ayaka Neko   | @nekomeowww    |

## License

MIT.
