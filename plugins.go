package plugins

import "code.cloudfoundry.org/cli/plugin"

type Plugin interface {
	Execute(CliConnection, []string)
	GetMetadata() plugin.PluginMetadata
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

type pluginConverter struct {
	plugin Plugin
}

func (p *pluginConverter) Run(cliConnection plugin.CliConnection, args []string) {
	connection, e := newCliConnection(cliConnection)
	if e != nil {
		panic(e)
	}
	p.plugin.Execute(connection, args)
}

func (p *pluginConverter) GetMetadata() plugin.PluginMetadata {
	return p.plugin.GetMetadata()
}

func Start(cmd plugin.Plugin) {
	plugin.Start(&pluginWrapper{cmd})
}

func Execute(cmd Plugin) {
	plugin.Start(&pluginConverter{cmd})
}
