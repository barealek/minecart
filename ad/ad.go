package ad

import "math/rand"

var ads = []string{
	"§7Get your server at §bPlexHost§7!",
	"§7Powered by §bPlexHost§7!",
	"§7Visit §bPlexHost.com§7 for more info!",
	"§7Hosting your server with §bPlexHost§7 is easy!",
}

func ChooseAd() string {
	if len(ads) == 0 {
		return "No ads available"
	}
	n := len(ads)
	s := rand.Intn(n)
	return ads[s]
}
