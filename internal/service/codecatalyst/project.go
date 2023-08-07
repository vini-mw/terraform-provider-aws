// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package codecatalyst

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codecatalyst"
	"github.com/aws/aws-sdk-go-v2/service/codecatalyst/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/create"
	"github.com/hashicorp/terraform-provider-aws/internal/errs"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/names"
)

// Function annotations are used for resource registration to the Provider. DO NOT EDIT.
// @SDKResource("aws_codecatalyst_project", name="Project")
func ResourceProject() *schema.Resource {
	return &schema.Resource{

		CreateWithoutTimeout: resourceProjectCreate,
		ReadWithoutTimeout:   resourceProjectRead,
		UpdateWithoutTimeout: resourceProjectUpdate,
		DeleteWithoutTimeout: resourceProjectDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"space_name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"display_name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"name": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

const (
	ResNameProject = "Project"
)

func resourceProjectCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	conn := meta.(*conns.AWSClient).CodeCatalystClient(ctx)

	in := &codecatalyst.CreateProjectInput{
		DisplayName: aws.String(d.Get("display_name").(string)),
		SpaceName:   aws.String(d.Get("space_name").(string)),
		Description: aws.String(d.Get("description").(string)),
	}

	out, err := conn.CreateProject(ctx, in)
	if err != nil {
		return create.DiagError(names.CodeCatalyst, create.ErrActionCreating, ResNameProject, d.Get("display_name").(string), err)
	}

	if out == nil || out.Name == nil {
		return create.DiagError(names.CodeCatalyst, create.ErrActionCreating, ResNameProject, d.Get("display_name").(string), errors.New("empty output"))
	}

	d.SetId(aws.ToString(out.Name))
	return resourceProjectRead(ctx, d, meta)
}

func resourceProjectRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	conn := meta.(*conns.AWSClient).CodeCatalystClient(ctx)

	spaceName := aws.String(d.Get("space_name").(string))

	out, err := findProjectByName(ctx, conn, d.Id(), spaceName)

	if !d.IsNewResource() && tfresource.NotFound(err) {
		log.Printf("[WARN] CodeCatalyst Project (%s) not found, removing from state", d.Id())
		d.SetId("")
		return diags
	}

	if err != nil {
		return append(diags, create.DiagError(names.CodeCatalyst, create.ErrActionReading, ResNameProject, d.Id(), err)...)
	}

	d.Set("name", out.Name)
	d.Set("space_name", out.SpaceName)
	d.Set("description", out.Description)

	return diags
}

func resourceProjectUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	conn := meta.(*conns.AWSClient).CodeCatalystClient(ctx)

	update := false

	in := &codecatalyst.UpdateProjectInput{
		Name:        aws.String(d.Get("display_name").(string)),
		SpaceName:   aws.String(d.Get("space_name").(string)),
		Description: aws.String(d.Get("description").(string)),
	}

	if d.HasChanges("description") {
		in.Description = aws.String(d.Get("description").(string))
		update = true
	}

	if !update {
		return diags
	}

	log.Printf("[DEBUG] Updating Codecatalyst Project (%s): %#v", d.Id(), in)

	_, err := conn.UpdateProject(ctx, in)
	if err != nil {
		return append(diags, create.DiagError(names.CodeCatalyst, create.ErrActionUpdating, ResNameProject, d.Id(), err)...)
	}

	return append(diags, resourceDevEnvironmentRead(ctx, d, meta)...)

}

func resourceProjectDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).CodeCatalystClient(ctx)

	log.Printf("[INFO] Deleting CodeCatalyst Project %s", d.Id())

	_, err := conn.DeleteProject(ctx, &codecatalyst.DeleteProjectInput{
		Name:      aws.String(d.Id()),
		SpaceName: aws.String(d.Get("space_name").(string)),
	})
	if err != nil {
		var nfe *types.ResourceNotFoundException
		if errors.As(err, &nfe) {
			return nil
		}

		return create.DiagError(names.CodeCatalyst, create.ErrActionDeleting, ResNameProject, d.Id(), err)
	}

	return nil
}

func statusProject(ctx context.Context, conn *codecatalyst.Client, id string, spaceName *string) retry.StateRefreshFunc {
	return func() (interface{}, string, error) {
		out, err := findProjectByName(ctx, conn, id, spaceName)
		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return out, aws.ToString(out.Name), nil
	}
}

func findProjectByName(ctx context.Context, conn *codecatalyst.Client, id string, spaceName *string) (*codecatalyst.GetProjectOutput, error) {
	in := &codecatalyst.GetProjectInput{
		Name:      aws.String(id),
		SpaceName: spaceName,
	}
	out, err := conn.GetProject(ctx, in)
	if errs.IsA[*types.AccessDeniedException](err) || errs.IsA[*types.ResourceNotFoundException](err) {
		return nil, &retry.NotFoundError{
			LastError:   err,
			LastRequest: in,
		}
	}
	if err != nil {
		return nil, err
	}

	if out == nil || out.Name == nil {
		return nil, tfresource.NewEmptyResultError(in)
	}

	return out, nil
}
