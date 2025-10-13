package plugins

import "code.cloudfoundry.org/cli/plugin"

type Plugin interface {
	plugin.Plugin
}

type pluginWrapper struct {
	plugin plugin.Plugin
}

func (p *pluginWrapper) Run(cliConnection plugin.CliConnection, args []string) {
	connection, e := newCliConnection(cliConnection)
	if e != nil {
		panic(e)
	}
	p.plugin.Run(connection, args)
}

func (p *pluginWrapper) GetMetadata() plugin.PluginMetadata {
	return p.plugin.GetMetadata()
}

func Start(cmd plugin.Plugin) {
	plugin.Start(&pluginWrapper{cmd})
}
