package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"code.cloudfoundry.org/cli/v8/plugin"
	plugin_models "code.cloudfoundry.org/cli/v8/plugin/models"
	"github.com/cloudfoundry/go-cfclient/v3/client"
	"github.com/cloudfoundry/go-cfclient/v3/config"
)

type CliConnection interface {
	plugin.CliConnection
	CfClient() *client.Client
}

type cliConnection struct {
	client     *client.Client
	connection plugin.CliConnection
}

func (c *cliConnection) CfClient() *client.Client {
	return c.client
}

func (c *cliConnection) CliCommandWithoutTerminalOutput(args ...string) ([]string, error) {
	return c.connection.CliCommandWithoutTerminalOutput(args...)
}

func (c *cliConnection) CliCommand(args ...string) ([]string, error) {
	return c.connection.CliCommand(args...)
}

func (c *cliConnection) GetCurrentOrg() (plugin_models.Organization, error) {
	return c.connection.GetCurrentOrg()
}

func (c *cliConnection) GetCurrentSpace() (plugin_models.Space, error) {
	return c.connection.GetCurrentSpace()
}

func (c *cliConnection) Username() (string, error) {
	return c.connection.Username()
}

func (c *cliConnection) UserGuid() (string, error) {
	return c.connection.UserGuid()
}

func (c *cliConnection) UserEmail() (string, error) {
	return c.connection.UserEmail()
}

func (c *cliConnection) IsLoggedIn() (bool, error) {
	return c.connection.IsLoggedIn()
}

func (c *cliConnection) IsSSLDisabled() (bool, error) {
	return c.connection.IsSSLDisabled()
}

func (c *cliConnection) HasOrganization() (bool, error) {
	return c.connection.HasOrganization()
}

func (c *cliConnection) HasSpace() (bool, error) {
	return c.connection.HasSpace()
}

func (c *cliConnection) ApiEndpoint() (string, error) {
	return c.connection.ApiEndpoint()
}

func (c *cliConnection) ApiVersion() (string, error) {
	return c.connection.ApiVersion()
}

func (c *cliConnection) HasAPIEndpoint() (bool, error) {
	return c.connection.HasAPIEndpoint()
}

func (c *cliConnection) LoggregatorEndpoint() (string, error) {
	return c.connection.LoggregatorEndpoint()
}

func (c *cliConnection) DopplerEndpoint() (string, error) {
	return c.connection.DopplerEndpoint()
}

func (c *cliConnection) AccessToken() (string, error) {
	return c.connection.AccessToken()
}

func (c *cliConnection) GetApp(name string) (plugin_models.GetAppModel, error) {
	spaceGuid, e := c.GetCurrentSpace()
	if e != nil {
		return plugin_models.GetAppModel{}, e
	}

	app, e := c.client.Applications.Single(context.Background(), &client.AppListOptions{
		Names:      client.Filter{Values: []string{name}},
		SpaceGUIDs: client.Filter{Values: []string{spaceGuid.Guid}},
	})
	if e != nil {
		return plugin_models.GetAppModel{}, e
	}

	result := plugin_models.GetAppModel{
		Guid:                 app.GUID,
		Name:                 name,
		Command:              "",
		DetectedStartCommand: "",
		EnvironmentVars:      make(map[string]any),
		State:                app.State,
		SpaceGuid:            spaceGuid.Guid,
	}

	env, e := c.client.Applications.GetEnvironmentVariables(context.Background(), app.GUID)
	if e != nil {
		return result, e
	}
	for k, v := range env {
		result.EnvironmentVars[k] = v
	}

	pkgs, e := c.client.Packages.ListForAppAll(context.Background(), app.GUID, nil)
	if e != nil {
		return result, e
	}
	// let's assume that the latest package is the last in the list
	pkg := pkgs[len(pkgs)-1]
	result.PackageUpdatedAt = &pkg.UpdatedAt

	droplet, e := c.client.Droplets.GetCurrentForApp(context.Background(), app.GUID)
	if e != nil {
		return result, e
	}
	// seems like summary returns what now shows as the droplet state
	result.PackageState = string(droplet.State)
	result.StagingFailedReason = emptyIfNil(droplet.Error)
	// v2 was returning a single buildpack name... multiple buildpacks were not shown previously
	result.BuildpackUrl = droplet.Buildpacks[0].Name

	stack, e := c.client.Stacks.Single(context.Background(), &client.StackListOptions{
		Names: client.Filter{Values: []string{droplet.Stack}},
	})
	if e != nil {
		return result, e
	}
	result.Stack = &plugin_models.GetApp_Stack{
		Guid:        stack.GUID,
		Name:        stack.Name,
		Description: emptyIfNil(stack.Description),
	}

	processes, _, e := c.client.Processes.ListForApp(context.Background(), app.GUID, nil)
	if e != nil {
		return result, e
	}
	process, e := c.client.Processes.Get(context.Background(), processes[0].GUID)
	if e != nil {
		return result, e
	}
	// the summary endpoint is no more compatible with apps supporting multiple process types, take just the first one
	result.DiskQuota = int64(process.DiskInMB)
	result.InstanceCount = process.Instances
	result.Memory = int64(process.MemoryInMB)
	result.HealthCheckTimeout = nullIfNil(process.HealthCheck.Data.Timeout)

	result.Command = emptyIfNil(process.Command)

	stats, e := c.client.Processes.GetStats(context.Background(), processes[0].GUID)
	if e != nil {
		return result, e
	}
	result.RunningInstances = len(stats.Stats)
	result.Instances = make([]plugin_models.GetApp_AppInstanceFields, len(stats.Stats))
	for i, stat := range stats.Stats {
		result.Instances[i] = plugin_models.GetApp_AppInstanceFields{
			State:     stat.State,
			Details:   emptyIfNil(stat.Details),
			Since:     time.Now().Add(-(time.Second * time.Duration(stat.Uptime))),
			CpuUsage:  stat.Usage.CPU,
			DiskQuota: int64(stat.DiskQuota),
			DiskUsage: int64(stat.Usage.Disk),
			MemQuota:  int64(stat.MemoryQuota),
			MemUsage:  int64(stat.Usage.Memory),
		}
	}

	routes, e := c.client.Routes.ListForAppAll(context.Background(), app.GUID, nil)
	if e != nil {
		return result, e
	}
	result.Routes = make([]plugin_models.GetApp_RouteSummary, len(routes))
	for i, route := range routes {
		result.Routes[i] = plugin_models.GetApp_RouteSummary{
			Guid: route.GUID,
			Host: route.Host,
			Domain: plugin_models.GetApp_DomainFields{
				Guid: route.Relationships.Domain.Data.GUID,
			},
			Path: route.Path,
			Port: nullIfNil(route.Port),
		}
		// avoid calling yet another api endpoint and compute the domain name
		if slashIndex := strings.Index(route.URL, "/"); slashIndex != -1 {
			result.Routes[i].Domain.Name = route.URL[len(route.Host)+1 : slashIndex]
		} else {
			result.Routes[i].Domain.Name = route.URL[len(route.Host)+1:]
		}
	}

	_, sis, e := c.client.ServiceCredentialBindings.ListIncludeServiceInstancesAll(context.Background(), &client.ServiceCredentialBindingListOptions{
		AppGUIDs: client.Filter{Values: []string{app.GUID}},
	})
	if e != nil {
		return result, e
	}
	result.Services = make([]plugin_models.GetApp_ServiceSummary, len(sis))
	for i, si := range sis {
		result.Services[i] = plugin_models.GetApp_ServiceSummary{
			Guid: si.GUID,
			Name: si.Name,
		}
	}

	return result, nil
}

func (c *cliConnection) GetApps() ([]plugin_models.GetAppsModel, error) {
	apps, e := c.client.Applications.ListAll(context.Background(), nil)
	if e != nil {
		return nil, e
	}

	result := make([]plugin_models.GetAppsModel, len(apps))
	for i, app := range apps {
		result[i] = plugin_models.GetAppsModel{
			Name:  app.Name,
			Guid:  app.GUID,
			State: app.State,
		}
	}

	return result, nil
}

func (c *cliConnection) GetOrgs() ([]plugin_models.GetOrgs_Model, error) {
	orgs, e := c.client.Organizations.ListAll(context.Background(), nil)
	if e != nil {
		return nil, e
	}

	result := make([]plugin_models.GetOrgs_Model, len(orgs))
	for i, org := range orgs {
		result[i] = plugin_models.GetOrgs_Model{
			Guid: org.GUID,
			Name: org.Name,
		}
	}

	return result, nil
}

func (c *cliConnection) GetSpaces() ([]plugin_models.GetSpaces_Model, error) {
	spaces, e := c.client.Spaces.ListAll(context.Background(), nil)
	if e != nil {
		return nil, e
	}

	result := make([]plugin_models.GetSpaces_Model, len(spaces))
	for i, space := range spaces {
		result[i] = plugin_models.GetSpaces_Model{
			Guid: space.GUID,
			Name: space.Name,
		}
	}

	return result, nil
}

func (c *cliConnection) GetOrgUsers(orgName string, args ...string) ([]plugin_models.GetOrgUsers_Model, error) {
	org, e := c.client.Organizations.Single(context.Background(), &client.OrganizationListOptions{
		Names: client.Filter{Values: []string{orgName}},
	})
	if e != nil {
		return nil, e
	}

	roles, users, e := c.client.Roles.ListIncludeUsersAll(context.Background(), &client.RoleListOptions{
		ListOptions:       nil,
		OrganizationGUIDs: client.Filter{Values: []string{org.GUID}},
	})
	if e != nil {
		return nil, e
	}

	// the args can only be empty (nil) or either --a or --all-users
	allUsers := true
	if len(args) > 1 {
		return nil, errors.New("too many arguments for organization users")
	} else if len(roles) == 1 {
		if args[0] != "-a" && args[0] != "--all-users" {
			return nil, errors.New("invalid arguments for organization users")
		}
		allUsers = false
	}

	userDict := make(map[string][]string)
	for _, role := range roles {
		if allUsers || role.Type != "organization_user" {
			userDict[role.Relationships.User.Data.GUID] = append(userDict[role.Relationships.User.Data.GUID], role.Type)
		}
	}

	var result []plugin_models.GetOrgUsers_Model
	for _, user := range users {
		if userRoles := userDict[user.GUID]; userRoles != nil {
			result = append(result, plugin_models.GetOrgUsers_Model{
				Guid:     user.GUID,
				Username: emptyIfNil(user.Username),
				Roles:    userRoles,
			})
		}
	}

	return result, nil
}

func (c *cliConnection) GetSpaceUsers(orgName string, spaceName string) ([]plugin_models.GetSpaceUsers_Model, error) {
	org, e := c.client.Organizations.Single(context.Background(), &client.OrganizationListOptions{
		Names: client.Filter{Values: []string{orgName}},
	})
	if e != nil {
		return nil, e
	}

	space, e := c.client.Spaces.Single(context.Background(), &client.SpaceListOptions{
		Names:             client.Filter{Values: []string{spaceName}},
		OrganizationGUIDs: client.Filter{Values: []string{org.GUID}},
	})
	if e != nil {
		return nil, e
	}

	roles, users, e := c.client.Roles.ListIncludeUsersAll(context.Background(), &client.RoleListOptions{
		SpaceGUIDs: client.Filter{Values: []string{space.GUID}},
	})
	if e != nil {
		return nil, e
	}

	userDict := make(map[string][]string)
	for _, role := range roles {
		userDict[role.Relationships.User.Data.GUID] = append(userDict[role.Relationships.User.Data.GUID], role.Type)
	}

	result := make([]plugin_models.GetSpaceUsers_Model, len(users))
	for i, user := range users {
		result[i] = plugin_models.GetSpaceUsers_Model{
			Guid:     user.GUID,
			Username: emptyIfNil(user.Username),
			Roles:    userDict[user.GUID],
		}
	}

	return result, nil
}

func (c *cliConnection) GetServices() ([]plugin_models.GetServices_Model, error) {
	currentSpace, e := c.GetCurrentSpace()
	if e != nil {
		return nil, e
	}

	serviceInstances, e := c.client.ServiceInstances.ListAll(context.Background(), &client.ServiceInstanceListOptions{
		SpaceGUIDs: client.Filter{Values: []string{currentSpace.Guid}},
	})
	if e != nil {
		return nil, e
	}

	var results []plugin_models.GetServices_Model
	for _, serviceInstance := range serviceInstances {
		result := plugin_models.GetServices_Model{
			Guid: serviceInstance.GUID,
			Name: serviceInstance.Name,
			LastOperation: plugin_models.GetServices_LastOperation{
				Type:  serviceInstance.LastOperation.Type,
				State: serviceInstance.LastOperation.State,
			},
			IsUserProvided: serviceInstance.Type == "user-provided",
		}

		_, apps, e := c.client.ServiceCredentialBindings.ListIncludeAppsAll(context.Background(), &client.ServiceCredentialBindingListOptions{
			ServiceInstanceGUIDs: client.Filter{Values: []string{serviceInstance.GUID}},
		})
		if e != nil {
			return nil, e
		}
		for _, app := range apps {
			result.ApplicationNames = append(result.ApplicationNames, app.Name)
		}

		servicePlan, e := c.client.ServicePlans.Get(context.Background(), serviceInstance.Relationships.ServicePlan.Data.GUID)
		if e != nil {
			return nil, e
		}
		result.ServicePlan = plugin_models.GetServices_ServicePlan{
			Name: servicePlan.Name,
			Guid: servicePlan.GUID,
		}

		serviceOffering, e := c.client.ServiceOfferings.Get(context.Background(), servicePlan.Relationships.ServiceOffering.Data.GUID)
		if e != nil {
			return nil, e
		}
		result.Service = plugin_models.GetServices_ServiceFields{
			Name: serviceOffering.Name,
		}

		results = append(results, result)
	}

	return results, nil
}

func (c *cliConnection) GetService(name string) (plugin_models.GetService_Model, error) {
	currentSpace, e := c.GetCurrentSpace()
	if e != nil {
		return plugin_models.GetService_Model{}, e
	}

	serviceInstance, e := c.client.ServiceInstances.Single(context.Background(), &client.ServiceInstanceListOptions{
		Names:      client.Filter{Values: []string{name}},
		SpaceGUIDs: client.Filter{Values: []string{currentSpace.Guid}},
	})
	if e != nil {
		return plugin_models.GetService_Model{}, e
	}

	result := plugin_models.GetService_Model{
		Guid:           serviceInstance.GUID,
		Name:           serviceInstance.Name,
		DashboardUrl:   emptyIfNil(serviceInstance.DashboardURL),
		IsUserProvided: serviceInstance.Type == "user-provided",
		LastOperation: plugin_models.GetService_LastOperation{
			Type:        serviceInstance.LastOperation.Type,
			State:       serviceInstance.LastOperation.State,
			Description: serviceInstance.LastOperation.Description,
			CreatedAt:   serviceInstance.LastOperation.CreatedAt.String(),
			UpdatedAt:   serviceInstance.LastOperation.UpdatedAt.String(),
		},
	}

	servicePlan, e := c.client.ServicePlans.Get(context.Background(), serviceInstance.Relationships.ServicePlan.Data.GUID)
	if e != nil {
		return result, e
	}
	result.ServicePlan = plugin_models.GetService_ServicePlan{
		Name: servicePlan.Name,
		Guid: servicePlan.GUID,
	}

	serviceOffering, e := c.client.ServiceOfferings.Get(context.Background(), servicePlan.Relationships.ServiceOffering.Data.GUID)
	if e != nil {
		return result, e
	}
	result.ServiceOffering = plugin_models.GetService_ServiceFields{
		Name:             serviceOffering.Name,
		DocumentationUrl: serviceOffering.DocumentationURL,
	}

	return result, nil
}

func (c *cliConnection) GetOrg(name string) (plugin_models.GetOrg_Model, error) {
	org, e := c.client.Organizations.Single(context.Background(), &client.OrganizationListOptions{
		Names: client.Filter{Values: []string{name}},
	})
	if e != nil {
		return plugin_models.GetOrg_Model{}, e
	}

	result := plugin_models.GetOrg_Model{
		Guid:        org.GUID,
		Name:        org.Name,
		Spaces:      nil,
		SpaceQuotas: nil,
	}

	domains, e := c.client.Domains.ListForOrganizationAll(context.Background(), org.GUID, nil)
	if e != nil {
		return result, e
	}
	for _, domain := range domains {
		owningOrganization := ""
		shared := domain.Relationships.Organization.Data == nil
		if !shared {
			owningOrganization = domain.Relationships.Organization.Data.GUID
		}
		result.Domains = append(result.Domains, plugin_models.GetOrg_Domains{
			Guid:                   domain.GUID,
			Name:                   domain.Name,
			OwningOrganizationGuid: owningOrganization,
			Shared:                 shared,
		})
	}

	spaces, e := c.client.Spaces.ListAll(context.Background(), &client.SpaceListOptions{
		OrganizationGUIDs: client.Filter{Values: []string{org.GUID}},
	})
	if e != nil {
		return result, e
	}
	for _, space := range spaces {
		result.Spaces = append(result.Spaces, plugin_models.GetOrg_Space{
			Guid: space.GUID,
			Name: space.Name,
		})
	}

	spaceQuotas, e := c.client.SpaceQuotas.ListAll(context.Background(), &client.SpaceQuotaListOptions{
		OrganizationGUIDs: client.Filter{Values: []string{org.GUID}},
	})
	if e != nil {
		return result, e
	}
	for _, quota := range spaceQuotas {
		result.SpaceQuotas = append(result.SpaceQuotas, plugin_models.GetOrg_SpaceQuota{
			Guid:                    quota.GUID,
			Name:                    quota.Name,
			MemoryLimit:             nullIfNil64(quota.Apps.TotalMemoryInMB),
			InstanceMemoryLimit:     nullIfNil64(quota.Apps.PerProcessMemoryInMB),
			RoutesLimit:             nullIfNil(quota.Routes.TotalRoutes),
			ServicesLimit:           nullIfNil(quota.Services.TotalServiceInstances),
			NonBasicServicesAllowed: quota.Services.PaidServicesAllowed,
		})
	}

	if org.Relationships.Quota.Data != nil {
		orgQuota, e := c.client.OrganizationQuotas.Get(context.Background(), org.Relationships.Quota.Data.GUID)
		if e != nil {
			return result, e
		}

		result.QuotaDefinition = plugin_models.QuotaFields{
			Guid:                    orgQuota.GUID,
			Name:                    orgQuota.Name,
			MemoryLimit:             nullIfNil64(orgQuota.Apps.TotalMemoryInMB),
			InstanceMemoryLimit:     nullIfNil64(orgQuota.Apps.PerProcessMemoryInMB),
			RoutesLimit:             nullIfNil(orgQuota.Routes.TotalRoutes),
			ServicesLimit:           nullIfNil(orgQuota.Services.TotalServiceInstances),
			NonBasicServicesAllowed: orgQuota.Services.PaidServicesAllowed,
		}
	}

	return result, nil
}

func (c *cliConnection) GetSpace(name string) (plugin_models.GetSpace_Model, error) {
	space, e := c.client.Spaces.Single(context.Background(), &client.SpaceListOptions{
		Names: client.Filter{Values: []string{name}},
	})
	if e != nil {
		return plugin_models.GetSpace_Model{}, e
	}

	result := plugin_models.GetSpace_Model{
		GetSpaces_Model: plugin_models.GetSpaces_Model{
			Guid: space.GUID,
			Name: space.Name,
		},
	}
	spaceFilter := client.Filter{Values: []string{space.GUID}}

	org, e := c.client.Organizations.Get(context.Background(), space.Relationships.Organization.Data.GUID)
	result.Organization = plugin_models.GetSpace_Orgs{
		Guid: org.GUID,
		Name: org.Name,
	}
	if e != nil {
		return result, e
	}

	apps, e := c.client.Applications.ListAll(context.Background(), &client.AppListOptions{SpaceGUIDs: spaceFilter})
	if e != nil {
		return result, e
	}
	result.Applications = make([]plugin_models.GetSpace_Apps, len(apps))
	for i, app := range apps {
		result.Applications[i] = plugin_models.GetSpace_Apps{
			Name: app.Name,
			Guid: app.GUID,
		}
	}

	sis, e := c.client.ServiceInstances.ListAll(context.Background(), &client.ServiceInstanceListOptions{SpaceGUIDs: spaceFilter})
	if e != nil {
		return result, e
	}
	result.ServiceInstances = make([]plugin_models.GetSpace_ServiceInstance, len(sis))
	for i, si := range sis {
		result.ServiceInstances[i] = plugin_models.GetSpace_ServiceInstance{
			Guid: si.GUID,
			Name: si.Name,
		}
	}

	domains, e := c.client.Domains.ListAll(context.Background(), &client.DomainListOptions{
		OrganizationGUIDs: client.Filter{Values: []string{org.GUID}},
	})
	if e != nil {
		return result, e
	}
	result.Domains = make([]plugin_models.GetSpace_Domains, len(domains))
	for i, domain := range domains {
		result.Domains[i] = plugin_models.GetSpace_Domains{
			Guid:                   domain.GUID,
			Name:                   domain.Name,
			OwningOrganizationGuid: domain.Relationships.Organization.Data.GUID,
			Shared:                 true,
		}
	}

	sgs, e := c.client.SecurityGroups.ListAll(context.Background(), &client.SecurityGroupListOptions{RunningSpaceGUIDs: spaceFilter})
	if e != nil {
		return result, e
	}
	result.SecurityGroups = make([]plugin_models.GetSpace_SecurityGroup, len(sgs))
	for i, sg := range sgs {
		result.SecurityGroups[i] = plugin_models.GetSpace_SecurityGroup{
			Name:  sg.Name,
			Guid:  sg.GUID,
			Rules: transformToMap(sg.Rules),
		}
	}

	if space.Relationships.Quota.Data != nil {
		quota, e := c.client.SpaceQuotas.Get(context.Background(), space.Relationships.Quota.Data.GUID)
		if e != nil {
			return result, e
		}
		result.SpaceQuota = plugin_models.GetSpace_SpaceQuota{
			Guid:                    quota.GUID,
			Name:                    quota.Name,
			MemoryLimit:             nullIfNil64(quota.Apps.TotalMemoryInMB),
			InstanceMemoryLimit:     nullIfNil64(quota.Apps.PerProcessMemoryInMB),
			RoutesLimit:             nullIfNil(quota.Routes.TotalRoutes),
			ServicesLimit:           nullIfNil(quota.Services.TotalServiceInstances),
			NonBasicServicesAllowed: quota.Services.PaidServicesAllowed,
		}
	}

	return result, nil
}

func nullIfNil64(value *int) int64 {
	if value == nil {
		return 0
	}
	return int64(*value)
}

func nullIfNil(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func emptyIfNil(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func transformToMap[T any](items []T) []map[string]any {
	result := make([]map[string]any, len(items))
	for i, item := range items {
		bytes, _ := json.Marshal(item)
		var mappedItem map[string]any
		_ = json.Unmarshal(bytes, &mappedItem)
		result[i] = mappedItem
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func newCliConnection(connection plugin.CliConnection) (CliConnection, error) {
	apiUrl, e := connection.ApiEndpoint()
	if e != nil {
		return nil, e
	}

	token, e := connection.AccessToken()
	if e != nil {
		return nil, e
	}

	token = token[7:]
	if CfConfig, e := config.New(apiUrl, config.Token(token, ""), config.SkipTLSValidation(), config.UserAgent("cfs-plugin/1.0.9")); e != nil {
		return nil, e
	} else if CfClient, e := client.New(CfConfig); e != nil {
		return nil, e
	} else {
		return &cliConnection{CfClient, connection}, nil
	}

}
