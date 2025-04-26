package category

import (
	"fmt"
)

const preambleString string = "Du skal nå kategorisere et kjøp ut ifra en liste med kategorier og en beskrivelse på kjøpet. Dette er ET kjøp på EN butikk."

func getPrompt(params CategorizationParams) (string, error) {
	rows, err := params.DB.Query("SELECT name, ai_notes FROM categories")

	if err != nil {
		return "", err
	}

	defer rows.Close()

	categoryListString := ""
	var name string
	var aiNotes string

	for rows.Next() {
		rows.Scan(&name, &aiNotes)
		categoryListString = fmt.Sprintf("%v- %v", categoryListString, name)
		if aiNotes != "" {
			categoryListString = fmt.Sprintf("%v - \"%v\"", categoryListString, aiNotes)
		}

		categoryListString = fmt.Sprintf("%v\n", categoryListString)
	}

	const baseString string = `%v 
	%v
	Totalbeløpet på kjøpet er %v kroner.
	%v
	Beskrivelsen av kjøpet er: "%v". 
	
	Du skal returnere JSON i form: 
		%v
	med så mange elementer som nødvendig.

	- description: Denne kan være en tom string hvis du ikke har mulighet til å utdype mer enn kategorien. (f.eks "dagligvarer")
	- ambiguity_flag: Hvis noe med kjøpet/oppdelingen/beskrivelsen er uklart, fyll strenget med en begunnelse. Ellers la stingen være tom (""). 
	Men ikke bruk den for mye, du må tenke litt også. Husk å skrive på norsk!
	%s

	Her er listen av kategorier du kan velge mellom, category_name står først, og eventuelle notater deretter: 
	%v
	Bruk den kategorien som er mest spesifikk. Det er ekstremt viktig at du bruker nøyaktig riktig category_name. 
	
	Ting som IKKE skal loggføres: sparing og investering. Disse bare eksluderer du fra svaret.

	Svaret skal altså BARE være json, UTEN markdown formatering.
	`

	var sharedStatusDescription, jsonFormatString, mixTip, userInfo string

	if params.SharedMode == "mix" {
		userInfo = fmt.Sprintf("Personen som har betalt (og oppgitt beskrivelsen) heter %s, mens den andre personen heter %s", params.Buyer.Name, params.SharedWith.Name)
		sharedStatusDescription = "Deler av kjøpet skal deles med en annen person, mens den andre delen skal tas alene."
		jsonFormatString = "{\"ambiguity_flag\": <string, tomt om ikke aktivert>, \"spendings\":[{\"apportion_mode\":\"shared\"|\"alone\"|\"other\", \"category\": <category_name>, \"amount\": <amount>, \"description\":<description>}..]}"
		mixTip = `Hvis for eksempel et av to elementer er spesifisert som kjøpt alene, så MÅ det andre være delt.
		- apportion_mode: "alone" dersom personen tar kjøpet alene, "shared" hvis kjøpet deles likt med den andre, "other" hvis den andre skal ta hele beløpet.`
	} else {
		userInfo = ""
		sharedStatusDescription = ""
		jsonFormatString = "{ \"ambiguity_flag\": <string, empty if nothing>, \"spendings\":[{\"category\": <category_name>, \"amount\": <amount>, \"description\":<description>}..]}"
		mixTip = ""
	}

	return fmt.Sprintf(baseString, preambleString, userInfo, params.TotalAmount, sharedStatusDescription, params.Prompt, jsonFormatString, categoryListString, mixTip), nil
}
