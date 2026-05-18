package teammcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nex-crm/wuphf/internal/opendesign"
)

// openDesignClient is package-level so tests can inject a fake.
// Production uses opendesign.New("") which reads WUPHF_OPEN_DESIGN_URL.
var openDesignClient = func() *opendesign.Client { return opendesign.New("") }

// registerOpenDesignTools wires the open-design design-fallback tools onto
// the per-agent MCP server. Only agents that can legitimately ship visual
// work (designer, cmo, ceo) get these tools — wiring them up for everyone
// inflates the MCP schema and burns prompt tokens with no benefit.
//
// All tools degrade cleanly when the daemon isn't running: each call
// pings /api/health first and returns a one-line "daemon unreachable"
// message that the agent can relay to the human instead of retrying
// silently or surfacing a raw transport error.
func registerOpenDesignTools(server *mcp.Server) {
	mcp.AddTool(server,
		readOnlyTool(
			"open_design_list_skills",
			"List design skills available in the local open-design daemon (decks, posters, mobile mocks, dashboards, magazine layouts, …). Filter by `scenario` (design, marketing, operation, engineering, product, finance, hr, sale, personal) when you only need one bucket. Use this BEFORE open_design_create_artifact so you can pick the skill that matches the human's ask. Returns [{id, name, description, scenario, mode}]. Falls back gracefully when the daemon isn't running.",
		),
		handleOpenDesignListSkills,
	)

	mcp.AddTool(server,
		readOnlyTool(
			"open_design_list_templates",
			"List the brand-grade design templates open-design ships (Linear, Stripe, Vercel, Airbnb, Tesla, Notion, Anthropic, Apple, Cursor, Supabase, Figma, …). Pair with a skill to render a particular brand's look. Returns [{id, name, description, category}].",
		),
		handleOpenDesignListTemplates,
	)

	mcp.AddTool(server,
		officeWriteTool(
			"open_design_create_artifact",
			"Render a real design artifact via the open-design daemon — a deck, poster, landing page, dashboard mock, magazine layout, etc. Required: `prompt`. Optional: `skillId` (pick from open_design_list_skills) and `templateId` (pick from open_design_list_templates) to constrain style. Returns {id, previewUrl, status}. After the call, paste the previewUrl into the channel so the team can see the result. Use this when a local model alone isn't enough for the visual work (real pixels needed, not prose).",
		),
		handleOpenDesignCreateArtifact,
	)

	mcp.AddTool(server,
		readOnlyTool(
			"open_design_list_artifacts",
			"List artifacts created in this open-design workspace, so you can pick one to refresh or reference without re-rendering from scratch.",
		),
		handleOpenDesignListArtifacts,
	)

	mcp.AddTool(server,
		officeWriteTool(
			"open_design_refresh_artifact",
			"Re-run the skill on an existing artifact to produce a variation. Use this when the human asks 'try again' or 'show me another version' instead of calling open_design_create_artifact a second time — keeps the artifact history coherent.",
		),
		handleOpenDesignRefreshArtifact,
	)
}

type openDesignListSkillsInput struct {
	Scenario string `json:"scenario,omitempty"`
}

func handleOpenDesignListSkills(ctx context.Context, _ *mcp.CallToolRequest, in openDesignListSkillsInput) (*mcp.CallToolResult, any, error) {
	c := openDesignClient()
	if err := c.Health(ctx); err != nil {
		return openDesignDaemonDownMsg(err), nil, nil
	}
	skills, err := c.ListSkills(ctx, strings.TrimSpace(in.Scenario))
	if err != nil {
		return toolErrorMsg(err.Error()), nil, nil
	}
	body, err := json.MarshalIndent(map[string]any{"skills": skills}, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil, nil
}

func handleOpenDesignListTemplates(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	c := openDesignClient()
	if err := c.Health(ctx); err != nil {
		return openDesignDaemonDownMsg(err), nil, nil
	}
	templates, err := c.ListTemplates(ctx)
	if err != nil {
		return toolErrorMsg(err.Error()), nil, nil
	}
	body, err := json.MarshalIndent(map[string]any{"templates": templates}, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil, nil
}

type openDesignCreateArtifactInput struct {
	Prompt     string `json:"prompt"`
	SkillID    string `json:"skillId,omitempty"`
	TemplateID string `json:"templateId,omitempty"`
	Title      string `json:"title,omitempty"`
	ProjectID  string `json:"projectId,omitempty"`
}

func handleOpenDesignCreateArtifact(ctx context.Context, _ *mcp.CallToolRequest, in openDesignCreateArtifactInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return toolErrorMsg("prompt is required"), nil, nil
	}
	c := openDesignClient()
	if err := c.Health(ctx); err != nil {
		return openDesignDaemonDownMsg(err), nil, nil
	}
	art, err := c.CreateArtifact(ctx, opendesign.CreateArtifactInput{
		Prompt:     in.Prompt,
		SkillID:    in.SkillID,
		TemplateID: in.TemplateID,
		Title:      in.Title,
		ProjectID:  in.ProjectID,
	})
	if err != nil {
		return toolErrorMsg(err.Error()), nil, nil
	}
	if art.PreviewURL == "" && art.ID != "" {
		art.PreviewURL = c.PreviewURL(art.ID)
	}
	body, err := json.MarshalIndent(art, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil, nil
}

func handleOpenDesignListArtifacts(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	c := openDesignClient()
	if err := c.Health(ctx); err != nil {
		return openDesignDaemonDownMsg(err), nil, nil
	}
	arts, err := c.ListArtifacts(ctx)
	if err != nil {
		return toolErrorMsg(err.Error()), nil, nil
	}
	body, err := json.MarshalIndent(map[string]any{"artifacts": arts}, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil, nil
}

type openDesignRefreshArtifactInput struct {
	ArtifactID string `json:"artifactId"`
}

func handleOpenDesignRefreshArtifact(ctx context.Context, _ *mcp.CallToolRequest, in openDesignRefreshArtifactInput) (*mcp.CallToolResult, any, error) {
	id := strings.TrimSpace(in.ArtifactID)
	if id == "" {
		return toolErrorMsg("artifactId is required"), nil, nil
	}
	c := openDesignClient()
	if err := c.Health(ctx); err != nil {
		return openDesignDaemonDownMsg(err), nil, nil
	}
	art, err := c.RefreshArtifact(ctx, id)
	if err != nil {
		return toolErrorMsg(err.Error()), nil, nil
	}
	if art.PreviewURL == "" {
		art.PreviewURL = c.PreviewURL(id)
	}
	body, err := json.MarshalIndent(art, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil, nil
}

// openDesignDaemonDownMsg returns a single-sentence error the LLM can
// quote to the human. The wrapped errors.Is check keeps the message
// stable when the underlying transport error changes (DNS, connect,
// 5xx, …) — agents shouldn't have to parse net/http stringification.
func openDesignDaemonDownMsg(err error) *mcp.CallToolResult {
	if errors.Is(err, opendesign.ErrDaemonUnreachable) {
		return toolErrorMsg("open-design daemon is not running. Ask the human to start it (cd to the open-design repo, run `pnpm tools-dev`).")
	}
	return toolErrorMsg(err.Error())
}
