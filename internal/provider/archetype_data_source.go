// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/matt-FFFFFF/alzlib"
	"github.com/matt-FFFFFF/terraform-provider-alz/internal/alztypes"
	"github.com/matt-FFFFFF/terraform-provider-alz/internal/alzvalidators"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ArchetypeDataSource{}

func NewArchetypeDataSource() datasource.DataSource {
	return &ArchetypeDataSource{}
}

// ArchetypeDataSource defines the data source implementation.
type ArchetypeDataSource struct {
	alz *alzlib.AlzLib
}

// ArchetypeDataSourceModel describes the data source data model.
type ArchetypeDataSourceModel struct {
	Name          types.String                     `tfsdk:"name"`
	ParentId      types.String                     `tfsdk:"parent_id"`
	BaseArchetype types.String                     `tfsdk:"base_archetype"`
	DisplayName   types.String                     `tfsdk:"display_name"`
	Defaults      ArchetypeDataSourceModelDefaults `tfsdk:"defaults"`
}

type ArchetypeDataSourceModelDefaults struct {
	DefaultLocation      types.String `tfsdk:"location"`
	DefaultLAWorkspaceId types.String `tfsdk:"log_analytics_workspace_id"`
}

func (d *ArchetypeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_archetype"
}

func (d *ArchetypeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Archetype data source.",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "The management group name, forming part of the resource id.",
				Required:            true,
			},

			"parent_id": schema.StringAttribute{
				MarkdownDescription: "The parent management group name.",
				Required:            true,
			},

			"base_archetype": schema.StringAttribute{
				MarkdownDescription: "The base archetype name to use. This has been generated from the provider lib directories.",
				Required:            true,
			},

			"policy_assignments_to_remove": schema.ListAttribute{
				MarkdownDescription: "A list of policy assignment names to remove from the archetype.",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.List{
					listvalidator.UniqueValues(),
				},
			},

			"policy_definitions_to_remove": schema.ListAttribute{
				MarkdownDescription: "A list of policy definition names to remove from the archetype.",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.List{
					listvalidator.UniqueValues(),
				},
			},

			"policy_set_definitions_to_remove": schema.ListAttribute{
				MarkdownDescription: "A list of policy set definition names to remove from the archetype.",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.List{
					listvalidator.UniqueValues(),
				},
			},

			"role_definitions_to_remove": schema.ListAttribute{
				MarkdownDescription: "A list of role definition names to remove from the archetype.",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.List{
					listvalidator.UniqueValues(),
				},
			},

			"policy_assignments_to_add": schema.MapNestedAttribute{
				MarkdownDescription: "A map of policy assignments names to add to the archetype. The map key is the policy assignemnt name.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.MapNestedAttribute{
							Required: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"display_name": schema.StringAttribute{
										MarkdownDescription: "The policy assignment display name",
										Required:            true,
									},

									"policy_definition_name": schema.StringAttribute{
										MarkdownDescription: "The name of the policy definition. Must be in the AlzLib, if it is not use `policy_definition_id` instead. Conflicts with `policy_definition_id`.",
										Optional:            true,
										Validators: []validator.String{
											stringvalidator.ConflictsWith(path.MatchRelative().AtMapKey("policy_definition_id")),
										},
									},

									"policy_definition_id": schema.StringAttribute{
										MarkdownDescription: "The resource id of the policy definition. Conflicts with `policy_definition_name`.",
										Optional:            true,
										Validators: []validator.String{
											stringvalidator.ConflictsWith(path.MatchRelative().AtMapKey("policy_definition_id")),
										},
									},

									"enforcement_mode": schema.StringAttribute{
										MarkdownDescription: "The enforcement mode of the policy assignment. Must be one of `Default`, or `DoNotEnforce`.",
										Optional:            true,
										Validators: []validator.String{
											stringvalidator.OneOf("Default", "DoNotEnforce"),
										},
									},

									"identity": schema.StringAttribute{
										MarkdownDescription: "The identity type. Must be one of `SystemAssigned` or `UserAssigned`.",
										Optional:            true,
										Validators: []validator.String{
											stringvalidator.OneOf("SystemAssigned", "UserAssigned"),
										},
									},

									"identity_ids": schema.ListAttribute{
										MarkdownDescription: "A list of identity ids to assign to the policy assignment. Required if `identity` is `UserAssigned`.",
										Optional:            true,
										ElementType:         types.StringType,
										Validators: []validator.List{
											listvalidator.UniqueValues(),
											listvalidator.ValueStringsAre(
												alzvalidators.ArmTypeResourceId("Microsoft.ManagedIdentity", "userAssignedIdentities"),
												stringvalidator.AlsoRequires(path.MatchRelative().AtMapKey("identity")),
											),
										},
									},

									"non_compliance_message": schema.SetNestedAttribute{
										MarkdownDescription: "The non-compliance messages to use for the policy assignment.",
										Optional:            true,
										NestedObject: schema.NestedAttributeObject{
											Attributes: map[string]schema.Attribute{
												"message": schema.StringAttribute{
													MarkdownDescription: "The non-compliance message.",
													Required:            true,
												},

												"policy_definition_reference_id": schema.StringAttribute{
													MarkdownDescription: "The policy definition reference id (not the resource id) to use for the non compliance message. This references the definition within the policy set.",
													Optional:            true,
												},
											},
										},
									},

									"parameters": schema.StringAttribute{
										MarkdownDescription: "The parameters to use for the policy assignment. " +
											"**Note:** This is a JSON string, and not a map. This is because the parameter values have different types, which confuses the type system used by the provider sdk. " +
											"Use `jsonencode()` to construct the map. " +
											"The map keys must be strings, the values are `any` type.",
										CustomType: alztypes.PolicyParameterType{},
										Optional:   true,
									},
								},
							},
						},
					},
				},
			},

			"policy_definitions_to_add": schema.ListAttribute{
				MarkdownDescription: "A list of policy definition names to add to the archetype.",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.List{
					listvalidator.UniqueValues(),
				},
			},

			"policy_set_definitions_to_add": schema.ListAttribute{
				MarkdownDescription: "A list of policy set definition names to add to the archetype.",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.List{
					listvalidator.UniqueValues(),
				},
			},

			"role_definitions_to_add": schema.ListAttribute{
				MarkdownDescription: "A list of role definition names to add to the archetype.",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.List{
					listvalidator.UniqueValues(),
				},
			},

			"role_assignments_to_add": schema.MapNestedAttribute{
				MarkdownDescription: "A list of role definition names to add to the archetype.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"definition": schema.StringAttribute{
							MarkdownDescription: "The role definition name, or resource id.",
							Required:            true,
						},
						"object_id": schema.StringAttribute{
							MarkdownDescription: "The principal object id to assign.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$`), "The subscription id must be a valid lowercase UUID."),
							},
						},
					},
				},
			},

			"defaults": schema.MapNestedAttribute{
				MarkdownDescription: "Archetype default values",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"location": schema.StringAttribute{
							MarkdownDescription: "Default location",
							Required:            true,
						},
						"log_analytics_workspace_id": schema.StringAttribute{
							MarkdownDescription: "Default Log Analytics workspace id",
							Optional:            true,
							Validators: []validator.String{
								alzvalidators.ArmTypeResourceId("Microsoft.OperationalInsights", "workspaces"),
							},
						},
					},
				},
			},

			"subscription_ids": schema.ListAttribute{
				MarkdownDescription: "A list of subscription ids to add to the management group.",
				Optional:            true,
				ElementType:         types.StringType,
				Validators: []validator.List{
					listvalidator.ValueStringsAre(
						stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$`), "The subscription id must be a valid lowercase UUID."),
					),
				},
			},
		},
	}
}

func (d *ArchetypeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	alz, ok := req.ProviderData.(*alzlib.AlzLib)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *alzlib.AlzLib, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.alz = alz
}

func (d *ArchetypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ArchetypeDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := d.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read example, got error: %s", err))
	//     return
	// }

	// For the purposes of this example code, hardcoding a response value to
	// save into the Terraform state.
	data.ParentId = types.StringValue("example-id")

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
