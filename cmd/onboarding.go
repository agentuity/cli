package cmd

import "github.com/agentuity/go-common/tui"

const onboardingDocsURL = "https://agentuity.dev/"

func showAuthNextSteps() {
	body := tui.Paragraph(
		tui.Secondary("1. Run ")+tui.Command("create")+tui.Secondary(" to scaffold your first Agent project."),
		tui.Secondary("2. Run ")+tui.Command("dev")+tui.Secondary(" to develop locally, then ")+tui.Command("deploy"),
		tui.Secondary("3. Explore the docs, see samples, and more: ")+tui.Link("%s", onboardingDocsURL),
	)
	tui.ShowBanner("Keep building with Agentuity", body, false)
}
