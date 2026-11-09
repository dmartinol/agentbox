package tui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmartinol/agentbox/internal/app"
	"github.com/rivo/tview"
)

func (ui *UI) profileForm() tview.Primitive {
	return ui.profileFormFor(nil)
}

func (ui *UI) profileEditForm(existing app.Profile) tview.Primitive {
	return ui.profileFormFor(&existing)
}

func (ui *UI) profileFormFor(existing *app.Profile) tview.Primitive {
	images := ui.imageNames()
	models := ui.modelSuggestions()
	providers := ui.providerNames("")
	policies := ui.policyPaths()

	var (
		name     string
		agent    = "codex"
		image    string
		mode     = "standard"
		model    string
		provider string
		policy   string
		approval = "prompted"
		title    = " Create profile "
		args     []string
	)
	if existing != nil {
		name = existing.Name
		agent = existing.Agent
		image = existing.Image
		mode = existing.Mode
		model = existing.Model
		provider = existing.Provider
		policy = existing.Policy
		approval = existing.Approval
		args = append([]string(nil), existing.Args...)
		title = " Edit profile: " + name + " "
	}

	nameField := tview.NewInputField().
		SetLabel("Name").
		SetText(name).
		SetFieldWidth(30).
		SetChangedFunc(func(text string) { name = text })
	if existing != nil {
		nameField.SetDisabled(true)
	}
	agents := []string{"codex", "claude", "opencode"}
	form := tview.NewForm().
		AddFormItem(nameField).
		AddDropDown("Agent", agents, optionIndex(agents, agent), func(text string, _ int) { agent = text }).
		AddFormItem(autocompleteInput("Image", image, 60, images, func(text string) { image = text })).
		AddDropDown("Mode", []string{"standard", "enmaas"}, optionIndex([]string{"standard", "enmaas"}, mode), func(text string, _ int) { mode = text }).
		AddFormItem(autocompleteInput("Model", model, 50, models, func(text string) { model = text })).
		AddFormItem(autocompleteInput("Provider", provider, 40, providers, func(text string) { provider = text })).
		AddFormItem(autocompleteInput("Policy", policy, 80, policies, func(text string) { policy = text })).
		AddDropDown("Approval", []string{"prompted", "unattended"}, optionIndex([]string{"prompted", "unattended"}, approval), func(text string, _ int) { approval = text }).
		AddButton("Save", func() {
			ui.asyncTo(pageProfiles, func() error {
				if policy == "" {
					policy = filepath.Join(ui.app.Store.PolicyDir(), name+".yaml")
					if _, err := os.Stat(policy); os.IsNotExist(err) {
						if err := ui.app.Store.Ensure(); err != nil {
							return err
						}
						if err := os.WriteFile(policy, app.DefaultPolicy(), 0600); err != nil {
							return err
						}
					}
				} else if _, err := os.Stat(policy); err != nil {
					return fmt.Errorf("policy file does not exist: %s", policy)
				}
				return ui.app.Store.SaveProfile(app.Profile{
					Version:  1,
					Name:     name,
					Agent:    agent,
					Image:    image,
					Mode:     mode,
					Model:    model,
					Provider: provider,
					Policy:   policy,
					Approval: approval,
					Args:     args,
				})
			})
		}).
		AddButton("Cancel", func() { ui.show(pageProfiles) })
	form.SetBorder(true).SetTitle(title)
	return form
}

func (ui *UI) runForm() tview.Primitive {
	profiles := ui.profileNames()
	if len(profiles) == 0 {
		form := tview.NewForm().
			AddTextView("", "No saved profiles found. Create a profile first; it will provide the image, model, provider, and policy for a run.", 70, 3, false, false).
			AddButton("Create profile", func() { ui.show("profile-form") }).
			AddButton("Cancel", func() { ui.show(pageRuns) })
		form.SetBorder(true).SetTitle(" Start run ")
		return form
	}
	images := ui.imageNames()
	gateways := ui.gatewayNames()
	models := ui.modelSuggestions()
	if len(gateways) == 0 {
		gateways = []string{"openshell"}
	}

	const useProfile = "<None>"
	gateway := gateways[0]
	var (
		profile  string
		source   = "."
		image    string
		mode     string
		model    string
		approval string
		prompt   string
	)

	profileValues := tview.NewTextView().
		SetLabel("Profile values (RO)").
		SetScrollable(false).
		SetWrap(true).
		SetSize(5, 100)

	profileDropdown := tview.NewDropDown().SetLabel("Profile")
	sourceField := tview.NewInputField().SetLabel("Source checkout").SetText(source).SetFieldWidth(60)
	gatewayDropdown := tview.NewDropDown().SetLabel("Gateway").SetOptions(gateways, func(text string, _ int) {
		gateway = text
	})

	overrideValue := func(text string) string {
		if text == useProfile {
			return ""
		}
		return text
	}
	imageOptions := append([]string{useProfile}, images...)
	modelOptions := append([]string{useProfile}, models...)
	modeOptions := []string{useProfile, "standard", "enmaas"}
	approvalOptions := []string{useProfile, "prompted", "unattended"}

	imageDropdown := tview.NewDropDown().
		SetLabel("Image override").
		SetOptions(imageOptions, func(text string, _ int) { image = overrideValue(text) })
	modeDropdown := tview.NewDropDown().
		SetLabel("Mode override").
		SetOptions(modeOptions, func(text string, _ int) { mode = overrideValue(text) })
	modelDropdown := tview.NewDropDown().
		SetLabel("Model override").
		SetOptions(modelOptions, func(text string, _ int) { model = overrideValue(text) })
	approvalDropdown := tview.NewDropDown().
		SetLabel("Approval override").
		SetOptions(approvalOptions, func(text string, _ int) { approval = overrideValue(text) })

	resetOverrides := func() {
		imageDropdown.SetCurrentOption(0)
		modeDropdown.SetCurrentOption(0)
		modelDropdown.SetCurrentOption(0)
		approvalDropdown.SetCurrentOption(0)
		image, mode, model, approval = "", "", "", ""
	}
	setOverridesEnabled := func(enabled bool) {
		for _, dropdown := range []*tview.DropDown{imageDropdown, modeDropdown, modelDropdown, approvalDropdown} {
			dropdown.SetDisabled(!enabled)
		}
		if !enabled {
			resetOverrides()
		}
	}
	overrideCheckbox := tview.NewCheckbox().
		SetLabel("Override profile").
		SetChecked(false).
		SetChangedFunc(func(checked bool) { setOverridesEnabled(checked) })
	applyProfile := func(name string) {
		profile = name
		saved, err := ui.app.Store.LoadProfile(name)
		if err != nil {
			profileValues.SetText("(profile values unavailable)")
			resetOverrides()
			return
		}
		profileValues.SetText(profileValueSummary(saved))
		resetOverrides()
	}
	profileDropdown.SetOptions(profiles, func(text string, _ int) { applyProfile(text) })
	profileDropdown.SetCurrentOption(0)
	sourceField.SetChangedFunc(func(text string) { source = text })
	gatewayDropdown.SetCurrentOption(0)
	imageDropdown.SetCurrentOption(0)
	modeDropdown.SetCurrentOption(0)
	modelDropdown.SetCurrentOption(0)
	approvalDropdown.SetCurrentOption(0)
	setOverridesEnabled(false)

	form := tview.NewForm().
		AddFormItem(profileDropdown).
		AddFormItem(sourceField).
		AddFormItem(gatewayDropdown).
		AddFormItem(profileValues).
		AddInputField("Prompt", "", 80, nil, func(text string) { prompt = text }).
		AddFormItem(overrideCheckbox).
		AddTextView("Overrides", "Enable Override profile to edit.", 100, 1, false, false).
		AddFormItem(imageDropdown).
		AddFormItem(modeDropdown).
		AddFormItem(modelDropdown).
		AddFormItem(approvalDropdown).
		AddButton("Start", func() {
			ui.asyncTo(pageRuns, func() error {
				_, err := ui.app.Start(ui.ctx, app.StartOptions{
					Profile:  profile,
					Source:   source,
					Gateway:  gateway,
					Image:    image,
					Mode:     mode,
					Model:    model,
					Approval: approval,
					Prompt:   prompt,
				})
				return err
			})
		}).
		AddButton("Cancel", func() { ui.show(pageRuns) })
	form.SetBorder(true).SetTitle(" Start run ")
	form.SetItemPadding(0)
	return form
}

func profileValueSummary(p app.Profile) string {
	return fmt.Sprintf("Agent: %s\nMode: %s    Model: %s\nApproval: %s\nImage: %s\nProvider: %s",
		p.Agent, p.Mode, shortenValue(p.Model, 16), p.Approval, shortenValue(p.Image, 20), shortenValue(p.Provider, 20))
}

func shortenValue(value string, max int) string {
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return "..." + value[len(value)-(max-3):]
}

func (ui *UI) imageVersionForm() tview.Primitive {
	var agent = "codex"
	form := tview.NewForm().
		AddDropDown("Agent", []string{"codex", "claude", "opencode"}, 0, func(text string, _ int) { agent = text }).
		AddButton("Discover", func() {
			ui.asyncTo(pageImages, func() error {
				_, err := ui.app.ImageVersion(ui.ctx, agent)
				return err
			})
		}).
		AddButton("Cancel", func() { ui.show(pageImages) })
	form.SetBorder(true).SetTitle(" Discover image version ")
	return form
}

func (ui *UI) imageBuildForm() tview.Primitive {
	var agent, version = "codex", ""
	form := tview.NewForm().
		AddDropDown("Agent", []string{"codex", "claude", "opencode"}, 0, func(text string, _ int) { agent = text }).
		AddInputField("Exact version", "", 20, nil, func(text string) { version = text }).
		AddButton("Build", func() {
			ui.asyncTo(pageImages, func() error {
				_, err := ui.app.ImageBuild(ui.ctx, agent, version, false)
				return err
			})
		}).
		AddButton("Cancel", func() { ui.show(pageImages) })
	form.SetBorder(true).SetTitle(" Build image ")
	return form
}

func (ui *UI) imagePushForm() tview.Primitive {
	artifacts, err := ui.app.LocalImages(ui.ctx)
	if err != nil {
		return ui.errorPage(err)
	}
	if len(artifacts) == 0 {
		form := tview.NewForm().
			AddTextView("", "No local AgentBox images found. Build an image before pushing.", 70, 2, false, false).
			AddButton("Build image", func() { ui.show("image-build-form") }).
			AddButton("Cancel", func() { ui.show(pageImages) })
		form.SetBorder(true).SetTitle(" Push image ")
		return form
	}
	registry, err := ui.app.Registry()
	if err != nil {
		return ui.errorPage(err)
	}
	if registry.Reference == "" {
		form := tview.NewForm().
			AddTextView("", "No external registry configured. Configure it before pushing images.", 70, 2, false, false).
			AddButton("Registry", func() { ui.show(pageImageRegistry) }).
			AddButton("Cancel", func() { ui.show(pageImages) })
		form.SetBorder(true).SetTitle(" Push image ")
		return form
	}
	refs := make([]string, 0, len(artifacts))
	byReference := make(map[string]app.ImageArtifact, len(artifacts))
	for _, artifact := range artifacts {
		refs = append(refs, artifact.Reference)
		byReference[artifact.Reference] = artifact
	}
	var localTag, remote string
	remoteField := tview.NewInputField().SetLabel("Remote reference").SetFieldWidth(90)
	remoteField.SetDisabled(true)
	localField := tview.NewDropDown().
		SetLabel("Local tag").
		SetFieldWidth(70).
		SetOptions(refs, func(text string, _ int) {
			localTag = text
			if artifact, ok := byReference[text]; ok {
				remote = app.RemoteReference(registry.NormalizedReference(), artifact)
				remoteField.SetText(remote)
			}
		})
	localField.SetCurrentOption(0)
	form := tview.NewForm().
		AddFormItem(localField).
		AddFormItem(remoteField).
		AddButton("Push", func() {
			ui.asyncTo(pageImages, func() error { return ui.app.ImagePush(ui.ctx, localTag, remote, false) })
		}).
		AddButton("Cancel", func() { ui.show(pageImages) })
	form.SetBorder(true).SetTitle(" Push image ")
	return form
}

func (ui *UI) imagePullForm() tview.Primitive {
	var ref string
	form := tview.NewForm().
		AddFormItem(autocompleteInput("Remote reference", "", 90, ui.registryReferences(), func(text string) { ref = text })).
		AddButton("Pull", func() {
			ui.asyncTo(pageImages, func() error { return ui.app.ImagePull(ui.ctx, ref, false) })
		}).
		AddButton("Cancel", func() { ui.show(pageImages) })
	form.SetBorder(true).SetTitle(" Pull image ")
	return form
}

func (ui *UI) gatewayForm() tview.Primitive {
	var endpoint, name = "https://localhost:17670", "openshell"
	form := tview.NewForm().
		AddInputField("Endpoint URL", "https://localhost:17670", 60, nil, func(text string) { endpoint = text }).
		AddInputField("Name", "openshell", 30, nil, func(text string) { name = text }).
		AddButton("Connect", func() {
			ui.asyncTo(pageGateways, func() error { return ui.app.GatewayConnect(ui.ctx, endpoint, name, false) })
		}).
		AddButton("Cancel", func() { ui.show(pageGateways) })
	form.SetBorder(true).SetTitle(" Connect gateway ")
	return form
}

func (ui *UI) gatewayInfoForm() tview.Primitive {
	gateways := ui.gatewayNames()
	name := "openshell"
	if len(gateways) > 0 {
		name = gateways[0]
	}
	form := tview.NewForm().
		AddFormItem(autocompleteInput("Gateway name", name, 40, gateways, func(text string) { name = text })).
		AddButton("Inspect", func() {
			go func() {
				data, err := ui.app.GatewayInfo(ui.ctx, name)
				if err != nil {
					ui.tapp.QueueUpdateDraw(func() { ui.message("[red]Error: %s", err) })
					return
				}
				tree, err := jsonTree(data, "Gateway info")
				if err != nil {
					ui.tapp.QueueUpdateDraw(func() { ui.message("[red]Error: %s", err) })
					return
				}
				tree.SetBorder(true).SetTitle(" Gateway: " + name + " ")
				ui.tapp.QueueUpdateDraw(func() {
					ui.showDynamic("gateway-info:"+name, tree)
					ui.message("[green]Gateway inspected.")
				})
			}()
		}).
		AddButton("Cancel", func() { ui.show(pageGateways) })
	form.SetBorder(true).SetTitle(" Inspect gateway ")
	return form
}

func (ui *UI) providerForm() tview.Primitive {
	gateways := ui.gatewayNames()
	gateway := "openshell"
	if len(gateways) > 0 {
		gateway = gateways[0]
	}
	var kind, name = "enmaas-bearer", ""
	form := tview.NewForm().
		AddDropDown("Type", []string{"enmaas-bearer", "enmaas-header", "openai", "anthropic"}, 0, func(text string, _ int) { kind = text }).
		AddInputField("Provider name", "", 30, nil, func(text string) { name = text }).
		AddFormItem(autocompleteInput("Gateway", gateway, 40, gateways, func(text string) { gateway = text })).
		AddButton("Set up", func() {
			ui.asyncTo(pageProfiles, func() error { return ui.app.ProviderSetup(ui.ctx, kind, name, gateway, false, false) })
		}).
		AddButton("Cancel", func() { ui.show(pageProfiles) })
	form.SetBorder(true).SetTitle(" Set up provider ")
	return form
}

func (ui *UI) policyHostForm() tview.Primitive {
	runs, err := ui.app.Store.ListRuns()
	if err != nil {
		return ui.errorPage(err)
	}
	var runID, host, binary string
	runIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		runIDs = append(runIDs, run.ID)
	}
	defaultRun := ""
	if len(runIDs) > 0 {
		defaultRun = runIDs[0]
		runID = defaultRun
	}
	binaries := []string{"/usr/local/bin/claude", "/usr/local/bin/codex", "/usr/local/bin/opencode"}
	form := tview.NewForm().
		AddFormItem(autocompleteInput("Run ID", defaultRun, 40, uniqueSorted(runIDs), func(text string) { runID = text })).
		AddInputField("Host", "", 50, nil, func(text string) { host = text }).
		AddFormItem(autocompleteInput("Binary path", "", 70, binaries, func(text string) { binary = text })).
		AddButton("Allow", func() {
			ui.asyncTo(pagePolicies, func() error { return ui.app.PolicyEdit(ui.ctx, runID, host, binary, false) })
		}).
		AddButton("Cancel", func() { ui.show(pagePolicies) })
	form.SetBorder(true).SetTitle(" Allow network host ")
	return form
}
