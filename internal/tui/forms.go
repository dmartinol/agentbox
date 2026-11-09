package tui

import (
	"os"
	"path/filepath"

	"github.com/dmartinol/agentbox/internal/app"
	"github.com/rivo/tview"
)

func (ui *UI) profileForm() tview.Primitive {
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
	)
	form := tview.NewForm().
		AddInputField("Name", "", 30, nil, func(text string) { name = text }).
		AddDropDown("Agent", []string{"codex", "claude", "opencode"}, 0, func(text string, _ int) { agent = text }).
		AddFormItem(autocompleteInput("Image", "", 60, images, func(text string) { image = text })).
		AddDropDown("Mode", []string{"standard", "enmaas"}, 0, func(text string, _ int) { mode = text }).
		AddFormItem(autocompleteInput("Model", "", 50, models, func(text string) { model = text })).
		AddFormItem(autocompleteInput("Provider", "", 40, providers, func(text string) { provider = text })).
		AddFormItem(autocompleteInput("Policy", "", 80, policies, func(text string) { policy = text })).
		AddDropDown("Approval", []string{"prompted", "unattended"}, 0, func(text string, _ int) { approval = text }).
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
				})
			})
		}).
		AddButton("Cancel", func() { ui.show(pageProfiles) })
	form.SetBorder(true).SetTitle(" Create profile ")
	return form
}

func (ui *UI) runForm() tview.Primitive {
	profiles := ui.profileNames()
	images := ui.imageNames()
	gateways := ui.gatewayNames()
	models := ui.modelSuggestions()

	gateway := "openshell"
	if len(gateways) > 0 {
		gateway = gateways[0]
	}
	var (
		profile  string
		source   = "."
		image    string
		mode     string
		model    string
		approval string
		prompt   string
	)

	modeOptions := []string{"", "standard", "enmaas"}
	approvalOptions := []string{"", "prompted", "unattended"}

	modeDropdown := tview.NewDropDown().
		SetLabel("Mode override").
		SetOptions(modeOptions, func(text string, _ int) { mode = text }).
		SetCurrentOption(0)
	approvalDropdown := tview.NewDropDown().
		SetLabel("Approval override").
		SetOptions(approvalOptions, func(text string, _ int) { approval = text }).
		SetCurrentOption(0)

	profileField := autocompleteInput("Profile", "", 40, profiles, nil)
	sourceField := tview.NewInputField().SetLabel("Source checkout").SetText(source).SetFieldWidth(60)
	gatewayField := autocompleteInput("Gateway", gateway, 40, gateways, nil)
	imageField := autocompleteInput("Image override", "", 60, images, func(text string) { image = text })
	modelField := autocompleteInput("Model override", "", 50, models, func(text string) { model = text })
	profileField.SetChangedFunc(func(text string) {
		profile = text
		if saved, err := ui.app.Store.LoadProfile(text); err == nil {
			imageField.SetText(saved.Image)
			modelField.SetText(saved.Model)
			modeDropdown.SetCurrentOption(optionIndex(modeOptions, saved.Mode))
			approvalDropdown.SetCurrentOption(optionIndex(approvalOptions, saved.Approval))
		}
	})
	sourceField.SetChangedFunc(func(text string) { source = text })
	gatewayField.SetChangedFunc(func(text string) { gateway = text })
	if len(profiles) > 0 {
		profileField.SetText(profiles[0])
	}

	form := tview.NewForm().
		AddFormItem(profileField).
		AddFormItem(sourceField).
		AddFormItem(gatewayField).
		AddFormItem(imageField).
		AddFormItem(modeDropdown).
		AddFormItem(modelField).
		AddFormItem(approvalDropdown).
		AddInputField("Prompt", "", 80, nil, func(text string) { prompt = text }).
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
	return form
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
	images := ui.imageNames()
	var localTag, remote string
	form := tview.NewForm().
		AddFormItem(autocompleteInput("Local tag", "", 70, images, func(text string) { localTag = text })).
		AddInputField("Remote reference", "", 80, nil, func(text string) { remote = text }).
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
		AddInputField("Remote reference", "", 80, nil, func(text string) { ref = text }).
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
