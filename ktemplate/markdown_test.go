package ktemplate

import (
	"reflect"
	"testing"
)

func TestParseMarkdownTables(t *testing.T) {
	data := `
* [Transportation](#transportation)
* [URL Shorteners](#url-shorteners)
* [Vehicle](#vehicle)
* [Video](#video)
* [Weather](#weather)

### Animals
API | Description | Auth | HTTPS | CORS |
|---|---|---|---|---|
| [Axolotl](https://theaxolotlapi.netlify.app/) | Collection of axolotl pictures and facts | No | Yes | Unknown |
| [Cat Facts](https://alexwohlbruck.github.io/cat-facts/) | Daily cat facts | No | Yes | No |
| [Cataas](https://cataas.com/) | Cat as a service (cats pictures and gifs) | No | Yes | Unknown |
| [catAPI](https://github.com/ThatCopy/catAPI/wiki/Usage) | Random pictures of cats | No | Yes | Yes |
| [Cats](https://docs.thecatapi.com/) | Pictures of cats from Tumblr | apiKey | Yes | Unknown |
| [Dog Facts](https://dukengn.github.io/Dog-facts-API/) | Random dog facts | No | Yes | Unknown |
| [Dogs](https://dog.ceo/dog-api/) | Based on the Stanford Dogs Dataset | No | Yes | Yes |
| [HTTPCat](https://http.cat/) | Cat for every HTTP Status | No | Yes | Unknown |
| [IUCN](http://apiv3.iucnredlist.org/api/v3/docs) | IUCN Red List of Threatened Species | apiKey | No | Unknown |
| [Movebank](https://github.com/movebank/movebank-api-doc) | Movement and Migration data of animals | No | Yes | Unknown |
| [PlaceBear](https://placebear.com/) | Placeholder bear pictures | No | Yes | Yes |
| [PlaceDog](https://place.dog) | Placeholder Dog pictures | No | Yes | Yes |
| [RandomCat](https://aws.random.cat/meow) | Random pictures of cats | No | Yes | Yes |
| [RandomDog](https://random.dog/woof.json) | Random pictures of dogs | No | Yes | Yes |
| [RandomDuck](https://random-d.uk/api) | Random pictures of ducks | No | Yes | No |
| [RandomFox](https://randomfox.ca/floof/) | Random pictures of foxes | No | Yes | No |
| [RescueGroups](https://userguide.rescuegroups.org/display/APIDG/API+Developers+Guide+Home) | Adoption | No | Yes | Unknown |
| [Shibe.Online](http://shibe.online/) | Random pictures of Shiba Inu, cats or birds | No | Yes | Yes |

**[⬆ Back to Index](#index)**
### Anime
API | Description | Auth | HTTPS | CORS |
|---|---|---|---|---|
| [AniAPI](https://github.com/AniAPI-Team/AniAPI) | Anime discovery, streaming & syncing with trackers | OAuth | Yes | Yes |
| [AniList](https://github.com/AniList/ApiV2-GraphQL-Docs) | Anime discovery & tracking | OAuth | Yes | Unknown |
| [AnimeChan](https://github.com/RocktimSaikia/anime-chan) | Anime quotes (over 10k+) | No | Yes | No |
| [AnimeNewsNetwork](https://www.animenewsnetwork.com/encyclopedia/api.php) | Anime industry news | No | Yes | Yes |
| [Jikan](https://jikan.moe) | Unofficial MyAnimeList API | No | Yes | Yes |
| [Kitsu](https://kitsu.docs.apiary.io/) | Anime discovery platform | OAuth | Yes | Yes |
| [MyAnimeList](https://myanimelist.net/clubs.php?cid=13727) | Anime and Manga Database and Community | OAuth | Yes | Unknown |
| [Shikimori](https://shikimori.one/api/doc) | Anime discovery, tracking, forum, rates | OAuth | Yes | Unknown |
| [Studio Ghibli](https://ghibliapi.herokuapp.com) | Resources from Studio Ghibli films | No | Yes | Yes |
| [Waifu.pics](https://waifu.pics/docs) | Image sharing platform for anime images | No | Yes | No |

**[⬆ Back to Index](#index)**
### Anti-Malware
API | Description | Auth | HTTPS | CORS |
|---|---|---|---|---|
| [AbuseIPDB](https://docs.abuseipdb.com/) | IP/domain/URL reputation | apiKey | Yes | Unknown |
| [AlienVault Open Threat Exchange (OTX)](https://otx.alienvault.com/api) | IP/domain/URL reputation | apiKey | Yes | Unknown |
| [Google Safe Browsing](https://developers.google.com/safe-browsing/) | Google Link/Domain Flagging | apiKey | Yes | Unknown |
| [Metacert](https://metacert.com/) | Metacert Link Flagging | apiKey | Yes | Unknown |
| [URLhaus](https://urlhaus-api.abuse.ch/) | Bulk queries and Download Malware Samples | No | Yes | Unknown |
| [URLScan.io](https://urlscan.io/about-api/) | Scan and Analyse URLs | apiKey | Yes | Unknown |
| [VirusTotal](https://www.virustotal.com/en/documentation/public-api/) | VirusTotal File/URL Analysis | apiKey | Yes | Unknown |	
`

	t.Run("public-apis", func(t *testing.T) {
		f := NewFunctions(nil)

		tables := f.ParseMarkdownTables(data)

		if len(tables) != 3 {
			t.Errorf("expected 3 tables got %d", len(tables))
			return
		}

		table1 := tables[0]
		if !reflect.DeepEqual(table1.Columns, []string{"API", "Description", "Auth", "HTTPS", "CORS"}) {
			t.Errorf("invalid table columns for table 1: %v", table1.Columns)
		}
		if len(table1.Rows) != 18 {
			t.Errorf("invalid table rows length for table 1: %v", len(table1.Rows))
			return
		}
		if !reflect.DeepEqual(table1.Rows[5], []string{"https://dukengn.github.io/Dog-facts-API/", "Random doag facts", "No", "Yes", "Unknown"}) {
			t.Errorf("invalid table row 6: %v", table1.Rows[5])
		}
	})
}
