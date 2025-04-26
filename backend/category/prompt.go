package category

import (
	"database/sql"
	"fmt"
)

const preambleString string = "Du skal nå kategorisere et kjøp ut ifra en liste med kategorier og en beskrivelse på kjøpet. Dette er ET kjøp på EN butikk."

// getPrompt requires the db connection.
// CategorizationParams.SharedWith should be populated by the caller (handler) if a partner exists.
func getPrompt(db *sql.DB, params CategorizationParams) (string, error) {
	rows, err := db.Query("SELECT name, ai_notes FROM categories")

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

	// Updated baseString with clearer instructions for inferring apportion_mode
	const baseString string = `%v
%v
Totalbeløpet på kjøpet er %v kroner.
Beskrivelsen av kjøpet er: "%v".

Du skal dele opp kjøpet i en eller flere deler basert på beskrivelsen og totalbeløpet.
For HVER del skal du bestemme 'apportion_mode' basert KUN på beskrivelsen.
Du skal returnere JSON i formatet:
%v
med en eller flere elementer i "spendings"-listen.

VIKTIGE REGLER FOR 'apportion_mode':
%v

- ambiguity_flag: Hvis noe med kjøpet/oppdelingen/beskrivelsen er uklart, fyll strengen med en kort begrunnelse på norsk. Ellers la strengen være tom (""). Ikke bruk den for mye.
- description: Kan være tom string ("") hvis kategorien er beskrivende nok (f.eks. "Dagligvarer").

Eksempler på hvordan 'apportion_mode' skal bestemmes fra beskrivelsen:
1. Beskrivelse: "Redbull til meg for 25kr, og resten er delt middag", Totalbeløp: 100kr, Kjøper: %s, Partner: %s
   -> [{"apportion_mode":"alone", "category":"...", "amount":25.0, "description":"Redbull"}, {"apportion_mode":"shared", "category":"...", "amount":75.0, "description":"Middag"}]
2. Beskrivelse: "Billetter", Totalbeløp: 500kr, Kjøper: %s, Partner: %s
   -> [{"apportion_mode":"alone", "category":"...", "amount":500.0, "description":"Billetter"}] (Fordi deling ikke er nevnt)
3. Beskrivelse: "Brød til %s", Totalbeløp: 40kr, Kjøper: %s, Partner: %s
   -> [{"apportion_mode":"other", "category":"...", "amount":40.0, "description":"Brød"}] (Fordi det er spesifikt til partneren)
4. Beskrivelse: "Delt lunsj", Totalbeløp: 200kr, Kjøper: %s, Partner: %s
   -> [{"apportion_mode":"shared", "category":"...", "amount":200.0, "description":"Lunsj"}]
5. Beskrivelse: "Dagligvarer", Totalbeløp: 350kr, Kjøper: %s, Partner: %s
   -> [{"apportion_mode":"shared", "category":"Groceries", "amount":350.0, "description":"Dagligvarer"}] (Anta delt hvis partner finnes og ikke annet er spesifisert for felleskategorier som Groceries)
6. Beskrivelse: "Genser", Totalbeløp: 600kr, Kjøper: %s, Partner: %s
   -> [{"apportion_mode":"alone", "category":"Shopping", "amount":600.0, "description":"Genser"}] (Anta personlig hvis ikke annet er spesifisert for ting som klær)
7. Beskrivelse: "Flybilletter til oss", Totalbeløp: 2000kr, Kjøper: %s, Partner: %s
   -> [{"apportion_mode":"shared", "category":"Transport", "amount":2000.0, "description":"Flybilletter"}]

Her er listen av kategorier du kan velge mellom (category_name står først, notater etterpå):
%v
Bruk den mest spesifikke kategorien. Bruk NØYAKTIG riktig category_name.

Ekskluder sparing og investering fra svaret.
Svaret skal KUN være gyldig JSON, UTEN markdown-formatering. Summen av 'amount' i svaret MÅ være lik totalbeløpet.
`

	// User info string, always includes buyer, includes partner if available.
	var userInfo string
	var partnerNamePlaceholder string = "[Partner Name]" // Placeholder if partner exists but name couldn't be fetched
	if params.SharedWith != nil {
		partnerName := params.SharedWith.Name
		if partnerName == "" { // Handle case where partner exists but name fetch failed
			partnerName = partnerNamePlaceholder
		}
		userInfo = fmt.Sprintf("Personen som har betalt (og oppgitt beskrivelsen) heter %s. Kjøpet kan potensielt involvere partneren %s.", params.Buyer.Name, partnerName)
	} else {
		userInfo = fmt.Sprintf("Personen som har betalt (og oppgitt beskrivelsen) heter %s. Det er ingen partner involvert.", params.Buyer.Name)
	}

	// JSON format string remains the same
	const jsonFormatString string = `{"ambiguity_flag": "<string>", "spendings":[{"apportion_mode":"shared|alone|other", "category": "<category_name>", "amount": <float>, "description":"<string>"}]}`

	// Updated explanation of apportion_mode focusing on inference from the prompt
	const apportionModeExplanation string = `- "alone": Brukes når beskrivelsen indikerer at varen KUN er til kjøperen (%s), ELLER når ingenting om deling/partner er nevnt (standard antagelse for personlige ting som klær, billetter etc.).
- "shared": Brukes når beskrivelsen eksplisitt sier at varen er delt, felles, til "oss", ELLER når det er en typisk fellesutgift (som dagligvarer, middag ute) OG en partner (%s) finnes OG ingenting annet er spesifisert.
- "other": Brukes KUN når beskrivelsen eksplisitt sier at varen er KUN til partneren (%s).`

	var filledApportionModeExplanation string
	buyerName := params.Buyer.Name
	partnerName := partnerNamePlaceholder // Default placeholder
	if params.SharedWith != nil {
		if params.SharedWith.Name != "" {
			partnerName = params.SharedWith.Name
		}
		filledApportionModeExplanation = fmt.Sprintf(apportionModeExplanation, buyerName, partnerName, partnerName)
	} else {
		// Simplified explanation if no partner exists. AI should only use 'alone'.
		filledApportionModeExplanation = fmt.Sprintf("- \"alone\": Skal brukes for alle deler siden det ikke er noen partner å dele med.")
	}

	// Prepare example strings with actual names/placeholders
	exampleBuyerName := params.Buyer.Name
	examplePartnerName := "Partner" // Use generic name for examples unless specific one is available
	if params.SharedWith != nil && params.SharedWith.Name != "" {
		examplePartnerName = params.SharedWith.Name
	}


	// Construct the final prompt string
	return fmt.Sprintf(baseString,
		preambleString,
		userInfo,
		params.TotalAmount,
		params.Prompt,
		jsonFormatString,
		filledApportionModeExplanation,
		// Examples injected here
		exampleBuyerName, examplePartnerName, // Example 1
		exampleBuyerName, examplePartnerName, // Example 2
		examplePartnerName, exampleBuyerName, examplePartnerName, // Example 3
		exampleBuyerName, examplePartnerName, // Example 4
		exampleBuyerName, examplePartnerName, // Example 5
		exampleBuyerName, examplePartnerName, // Example 6
		exampleBuyerName, examplePartnerName, // Example 7
		// Category list at the end
		categoryListString), nil
}
