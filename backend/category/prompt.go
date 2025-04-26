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

	// Always include user info if shared person exists, regardless of mode hint.
	var userInfo string
	if params.SharedWith != nil {
		userInfo = fmt.Sprintf("Personen som har betalt (og oppgitt beskrivelsen) heter %s. Kjøpet kan potensielt deles med %s.", params.Buyer.Name, params.SharedWith.Name)
	} else {
		userInfo = fmt.Sprintf("Personen som har betalt (og oppgitt beskrivelsen) heter %s. Kjøpet er ikke delt med noen.", params.Buyer.Name)
	}

	// Always use the JSON format that includes apportion_mode.
	const jsonFormatString string = "{\"ambiguity_flag\": <string, tomt om ikke aktivert>, \"spendings\":[{\"apportion_mode\":\"shared\"|\"alone\"|\"other\", \"category\": <category_name>, \"amount\": <amount>, \"description\":<description>}..]}"

	// Always explain apportion_mode.
	const apportionModeExplanation string = `- apportion_mode: Angir hvordan hver del av kjøpet skal fordeles.
		- "alone": Kjøperen (%s) tar denne delen av kostnaden alene.
		- "shared": Kostnaden for denne delen deles likt mellom kjøperen (%s) og den andre personen (%s).
		- "other": Den andre personen (%s) skal dekke hele kostnaden for denne delen.`

	var filledApportionModeExplanation string
	if params.SharedWith != nil {
		filledApportionModeExplanation = fmt.Sprintf(apportionModeExplanation, params.Buyer.Name, params.Buyer.Name, params.SharedWith.Name, params.SharedWith.Name)
	} else {
		// Simplified explanation if there's no shared person involved. AI should only use 'alone'.
		filledApportionModeExplanation = fmt.Sprintf("- apportion_mode: Skal være \"alone\" siden kjøpet ikke deles.")
	}


	// Construct the final prompt string
	// Note: sharedStatusDescription is removed as the prompt now always handles potential mixes.
	return fmt.Sprintf(baseString, preambleString, userInfo, params.TotalAmount, "", params.Prompt, jsonFormatString, filledApportionModeExplanation, categoryListString), nil
}
