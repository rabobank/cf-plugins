# CF-plugins
A library to allow existing cf cli plugins to start using the v3 cc api instead of the v2 which is 
called through rpc calls. It only uses the rpc calls to obtain context information (
like current org and space, for example) and the cf token which can be used to
directly interact with the cf landscape.

___SINCE v1.00, cf-plugins uses the now compatible with golang version workflow, cli version.___
___Importing cf-plugins will now force the plugin to also update its dependency to the v8 cli version (code.cloudfoundry.org/cli => code.cloudfoundry.org/cli/v8)___

___v1+ is effectively a breaking change from the v0.* development versions___

## Usage

The library mimicks the normal cli plugins framework and emulates the plugin
CliConnection object, to migrate to v3 one must simply import the cf-plugins
library and use it to start the plugin instead of directly using the cf cli
plugin library, which means that for a simple plugin like:

```go
package main

import (
    "fmt"
    "os"

    "code.cloudfoundry.org/cli/v8/plugin"
)

type CfPlugin struct{}

func (c *CfPlugin) Run(cliConnection plugin.CliConnection, args []string) {
    if args[0] == "CLI-MESSAGE-UNINSTALL" {
        return
    }
    
    fmt.Println("I'm a plugin")
}

func (c *CfPlugin) GetMetadata() plugin.PluginMetadata {
    return plugin.PluginMetadata{
        Name:          "test-plugin",
        Version:       plugin.VersionType{Major: 1, Minor: 0, Build: 0},
        MinCliVersion: plugin.VersionType{Major: 8, Minor: 0, Build: 0},
        Commands:      []plugin.Command{
            {
                Name:     "tester",
                HelpText: "Simple test plugin",
            },
        },
    }
}

func main() {
    if len(os.Args) == 1 {
        _, _ = fmt.Fprintf(os.Stderr, "This executable is a cf plugin.\n"+
            "Run `cf install-plugin %s` to install it",
            os.Args[0])
        os.Exit(1)
    }

    plugin.Start(new(CfPlugin))
}
```

a simple import of cf-plugins and using the plugins Start method is enough to convert the plugin

```go
package main

import (
    "fmt"
    "os"

    "code.cloudfoundry.org/cli/v8/plugin"
    "github.com/rabobank/cf-plugins" // Add this import
)
                 .
                 .
                 .
func main() {
    if len(os.Args) == 1 {
        _, _ = fmt.Fprintf(os.Stderr, "This executable is a cf plugin.\n"+
            "Run `cf install-plugin %s` to install it",
            os.Args[0])
        os.Exit(1)
    }

    plugins.Start(new(CfPlugin)) // Switch to plugins instead of plugin
}
```

## Improved Usage

The CliConnection emulation is quite inefficient when using the standard CliConnection
methods to get app, service, orgs and spaces information. This is due to the more modularized
v3 api nature (as compared to v2) which requires several calls to provide the same information
the v2 app used to. Quite often, most of that information is not required so it would
be much more efficient to directly use the cc api to get the specific information.

To further migrate away from the standard cli plugin framework, you may explicitly implement
the cf-plugins interface and use Execute instead of Run to get a cli connection which
provides you a ccv3 api client which you may call to directly query cloud controller if
required.


```go
package main

import (
    "fmt"
    "os"

    "code.cloudfoundry.org/cli/v8/plugin"
    "github.com/rabobank/cf-plugins" // Add this import
)

type CfPlugin struct{}

func (c *CfPlugin) Execute(cliConnection plugins.CliConnection, args []string) {
    if args[0] == "CLI-MESSAGE-UNINSTALL" {
        return
    }
    
    fmt.Println("I'm a plugin")
}

func (c *CfPlugin) GetMetadata() plugin.PluginMetadata {
    return plugin.PluginMetadata{
        Name:          "test-plugin",
        Version:       plugin.VersionType{Major: 1, Minor: 0, Build: 0},
        MinCliVersion: plugin.VersionType{Major: 8, Minor: 0, Build: 0},
        Commands:      []plugin.Command{
            {
                Name:     "tester",
                HelpText: "Simple test plugin",
            },
        },
    }
}

func main() {
    if len(os.Args) == 1 {
        _, _ = fmt.Fprintf(os.Stderr, "This executable is a cf plugin.\n"+
            "Run `cf install-plugin %s` to install it",
            os.Args[0])
        os.Exit(1)
    }

    plugins.Execute(new(CfPlugin))
}
```

## Caveats

When using the emulated CliConnection to retrieve organization or space users, the flag IsAdmin will always be
set to false as to avoid having to query UAA to check the user roles.

The BuildpackUrl attribute of the App object when calling GetApp from the CliConnection will
return a single buildpack name.

The App object is not suitable for the now multiple-process supporting CF model, as such it will
be based on the first process type of the app

The list of apps will not get the additional usage statistics for every app, this
is also due to the multiple-process nature of the CF model which would require calling
the processes api for each app and use the first one.