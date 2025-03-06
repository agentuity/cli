package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

const logoHeader = `
                 ---                 
                -----                
               -------               
              ---- ----              
             ----   ----             
            ----     ----            
           ----       ----           
          ----------------------     
         ------------------------    


 -------------------------------     
---------------------------------    
   ----                       ----   
  ----                         ----  
 ----------------------------------- 
-------------------------------------
`

var (
	logoColor = lipgloss.AdaptiveColor{Light: "#36EEE0", Dark: "#00FFFF"}
	logoStyle = lipgloss.NewStyle().Foreground(logoColor)

	logoBox = lipgloss.NewStyle().
		Width(40).
		AlignVertical(lipgloss.Top).
		AlignHorizontal(lipgloss.Center).
		Foreground(bannerForegroupColor)
)

func Logo() {
	if !HasTTY {
		return
	}
	logo := logoStyle.Render(logoHeader)
	title := logoStyle.Render("Agentuity Agent Cloud")
	fmt.Println(logoBox.Render(logo + "\n" + title))
	fmt.Println()
}
