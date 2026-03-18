package cli

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/ohing504/devclean/internal/model"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List supported ecosystems and categories",
		RunE: func(cmd *cobra.Command, args []string) error {
			header := lipgloss.NewStyle().Bold(true)

			fmt.Println(header.Render("Ecosystems:"))
			for _, eco := range model.AllEcosystems() {
				fmt.Printf("  • %s\n", eco)
			}

			fmt.Println()
			fmt.Println(header.Render("Categories:"))
			for _, cat := range model.AllCategories() {
				fmt.Printf("  • %s\n", cat)
			}

			fmt.Println()
			fmt.Println(header.Render("Activity Status:"))
			fmt.Println("  • active   — modified within 7 days")
			fmt.Println("  • recent   — 7–30 days")
			fmt.Println("  • stale    — 30–90 days")
			fmt.Println("  • dormant  — 90+ days")

			fmt.Println()
			fmt.Println(header.Render("Safety Levels:"))
			fmt.Println("  • safe      — freely deletable, auto-regenerated")
			fmt.Println("  • caution   — deletable but may need rebuild")
			fmt.Println("  • protected — should not be deleted")

			return nil
		},
	}
}
