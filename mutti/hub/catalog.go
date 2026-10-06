// SPDX-License-Identifier: GPL-2.0-or-later
package hub

// CatalogModel is one curated, digest-pinned engine artifact. A tag alone is
// not a lock: Mutti only activates a model whose manifest digest matches.
type CatalogModel struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Family        string  `json:"family"`
	Digest        string  `json:"digest"`
	Bytes         int64   `json:"bytes"`
	MinMemoryGB   int     `json:"minMemoryGB"`
	Profile       string  `json:"profile"`
	Qualification string  `json:"qualification"`
	Note          string  `json:"note"`
	License       string  `json:"license"`
	Temperature   float64 `json:"-"`
	ContextTokens int     `json:"contextTokens"`
}

// Qualification values: "recommended" (passed the frozen casting gates for
// its hardware profile), "limited" (usable with documented weaknesses),
// "not_qualified" (installable for testing only, never preselected).
// Evidence: docs/mutti/evidence/model-casting-*.md.
var catalog = []CatalogModel{
	{ID: "qwen3.5:4b", Name: "Qwen 3.5 4B", Family: "qwen3.5", Digest: "d8b0f5e9760cd1682034f292d7ef72ec46f432149be0df7574bf2d6e92e38c04",
		Bytes: 3324173757, MinMemoryGB: 16, Profile: "kompakt", Qualification: "not_qualified", License: "Apache-2.0",
		Note: "Kleinster Kandidat; Qualifikation siehe Castingnachweis.", ContextTokens: 8192},
	{ID: "qwen3.5:9b", Name: "Qwen 3.5 9B", Family: "qwen3.5", Digest: "56671c2ab9385f9cfcb404638e32cd62d88e3501d44822208363c010179a3c90",
		Bytes: 6550825373, MinMemoryGB: 24, Profile: "ausgewogen", Qualification: "not_qualified", License: "Apache-2.0",
		Note: "Mittlerer Kandidat; Qualifikation siehe Castingnachweis.", ContextTokens: 8192},
	{ID: "gemma4:e4b", Name: "Gemma 4 E4B", Family: "gemma4", Digest: "dc35e8d9c6061baa6f0fa870975ab6932e2542b579b13ea0f199fa4bb7300c9c",
		Bytes: 6583656264, MinMemoryGB: 24, Profile: "ausgewogen", Qualification: "not_qualified", License: "Apache-2.0",
		Note: "Vergleich aus zweiter Modellfamilie; Qualifikation siehe Castingnachweis.", ContextTokens: 8192},
	{ID: "qwen3.6:35b-a3b", Name: "Qwen 3.6 35B-A3B", Family: "qwen3.6", Digest: "07d35212591fc27746f0a317c975a6d68754fb38e9053d82e25f06057af28522",
		Bytes: 23938333115, MinMemoryGB: 64, Profile: "hohe Qualität", Qualification: "not_qualified", License: "Apache-2.0",
		Note: "Großes Modell für starke Rechner; Qualifikation siehe Castingnachweis.", ContextTokens: 8192},
}

func catalogModel(id string) (CatalogModel, bool) {
	for _, m := range catalog {
		if m.ID == id {
			return m, true
		}
	}
	return CatalogModel{}, false
}
