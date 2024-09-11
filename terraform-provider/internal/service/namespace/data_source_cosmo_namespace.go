package namespace

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	platformv1 "github.com/wundergraph/cosmo/terraform-provider-cosmo/gen/proto/wg/cosmo/platform/v1"
	"github.com/wundergraph/cosmo/terraform-provider-cosmo/internal/api"
	"github.com/wundergraph/cosmo/terraform-provider-cosmo/internal/client"
	"github.com/wundergraph/cosmo/terraform-provider-cosmo/internal/utils"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &NamespaceDataSource{}

func NewNamespaceDataSource() datasource.DataSource {
	return &NamespaceDataSource{}
}

// NamespaceDataSource defines the data source implementation.
type NamespaceDataSource struct {
	*client.PlatformClient
}

// NamespaceDataSourceModel describes the data source data model.
type NamespaceDataSourceModel struct {
	Id   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (d *NamespaceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_namespace"
}

func (d *NamespaceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Cosmo Namespace Data Source",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The unique identifier of the namespace resource, automatically generated by the system.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the namespace.",
			},
		},
	}
}

func (d *NamespaceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*client.PlatformClient)
	if !ok {
		utils.AddDiagnosticError(resp, ErrUnexpectedDataSourceType, fmt.Sprintf("Expected *client.PlatformClient, got: %T. Please report this issue to the provider developers.", req.ProviderData))
		return
	}

	d.PlatformClient = client
}

func (d *NamespaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NamespaceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.Name.IsNull() || data.Name.ValueString() == "" {
		utils.AddDiagnosticError(resp, ErrInvalidNamespaceName, "The 'name' attribute is required.")
		return
	}

	namespaceData, err := api.ListNamespaces(ctx, d.PlatformClient.Client, d.PlatformClient.CosmoApiKey)
	if err != nil {
		utils.AddDiagnosticError(resp, ErrReadingNamespace, fmt.Sprintf("Could not read namespace: %s, name: %s, namespace: %s", err, data.Name.ValueString(), data.Name.ValueString()))
		return
	}

	var foundNamespace *platformv1.Namespace
	for _, ns := range namespaceData {
		if ns.Name == data.Name.ValueString() {
			foundNamespace = ns
			break
		}
	}

	if foundNamespace == nil {
		utils.AddDiagnosticError(resp, ErrRetrievingNamespace, fmt.Sprintf("Namespace with name '%s' not found", data.Name.ValueString()))
		return
	}

	data.Id = types.StringValue(foundNamespace.Name)
	data.Name = types.StringValue(foundNamespace.Name)

	tflog.Trace(ctx, "Read namespace data source", map[string]interface{}{
		"id": data.Id.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
