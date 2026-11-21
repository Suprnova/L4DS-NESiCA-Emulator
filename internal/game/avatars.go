package game

// Survivor is one of the four survivors, in the game's order: a card's chara_id is the survivor
// plus 1. client.dll lists them in this order (0x105b0d9c), and an avatar script's "only" key
// names them by the bits 1 << survivor. Confirmed for Sara, chara_id 2.
type Survivor int

const (
	Yusuke Survivor = iota
	Sara
	Haruka
	Jordan
)

// Survivors is the number of survivors.
const Survivors = 4

// SurvivorOf returns the survivor a card's chara_id names, and false for one that names none,
// such as the 0 of a card registered before characters were stored.
func SurvivorOf(charaID int64) (Survivor, bool) {
	if charaID < 1 || charaID > Survivors {
		return 0, false
	}
	return Survivor(charaID - 1), true
}

// AvatarSite is where an avatar item is worn, one of the card's avatar slots.
type AvatarSite int

const (
	// SiteHead is a hat or other head accessory (avator_head), worn over a hair of its own.
	SiteHead AvatarSite = iota
	// SiteUpper is clothing on the upper body (avator_tops).
	SiteUpper
	// SiteLower is clothing on the lower body (avator_bottoms).
	SiteLower
	// SiteFace is a face accessory (avator_acc_face).
	SiteFace
	// SiteAccUpper is an upper-body accessory (avator_acc_tops).
	SiteAccUpper
	// SiteAccLower is a lower-body accessory (avator_acc_bottoms).
	SiteAccLower
)

// AvatarItem is an avatar part or accessory of build 1.26 that a card can wear.
type AvatarItem struct {
	// Name is the item's setting name in the avatar scripts, less any survivor suffix:
	// "firefighter_upper_avt" stands for firefighter_upper_avt_y, _s, _h, and _j.
	Name string
	Site AvatarSite
	// Resources is the resource id each survivor wears the item as, from client.dll's tables
	// (avatar parts 0x1081e4d0, accessories 0x1081ea38); 0 for a survivor it does not fit. A
	// head item's is the card's head resource id (0x1081ecb8), the same for every survivor.
	Resources [Survivors]int
	// Hair is, for a head item, the hair resource each survivor wears under it, from the
	// script's per-survivor replacement; 0 for none.
	Hair [Survivors]int
}

// Avatar returns the avatar item named name.
func Avatar(name string) (AvatarItem, bool) {
	for _, item := range AvatarItems {
		if item.Name == name {
			return item, true
		}
	}
	return AvatarItem{}, false
}

// AvatarAt returns the avatar item survivor wears at site as resource.
func AvatarAt(site AvatarSite, survivor Survivor, resource int) (AvatarItem, bool) {
	for _, item := range AvatarItems {
		if item.Site == site && item.Resources[survivor] == resource {
			return item, true
		}
	}
	return AvatarItem{}, false
}

// AvatarItems are build 1.26's avatar items, from left4dead2\scripts\avataritem and client.dll,
// by site and name; hairs are left out, since a card's hair is not given as an item. A test
// compares them with an installed game when L4DS_GAME_DIR is set.
var AvatarItems = []AvatarItem{
	{Name: "acc00_adt_avt", Site: SiteHead, Resources: [Survivors]int{32, 32, 32, 32}, Hair: [Survivors]int{10, 11, 12, 9}},
	{Name: "acc01_adt_avt", Site: SiteHead, Resources: [Survivors]int{33, 33, 33, 33}, Hair: [Survivors]int{10, 31, 12, 9}},
	{Name: "acc02_adt_avt", Site: SiteHead, Resources: [Survivors]int{34, 34, 34, 34}, Hair: [Survivors]int{10, 31, 12, 9}},
	{Name: "acc03_adt_avt", Site: SiteHead, Resources: [Survivors]int{35, 35, 35, 35}, Hair: [Survivors]int{10, 11, 12, 9}},
	{Name: "acc04_adt_avt", Site: SiteHead, Resources: [Survivors]int{36, 36, 36, 36}, Hair: [Survivors]int{10, 11, 12, 9}},
	{Name: "acc05_adt_avt", Site: SiteHead, Resources: [Survivors]int{37, 37, 37, 37}, Hair: [Survivors]int{10, 11, 4, 9}},
	{Name: "acc06_adt_avt", Site: SiteHead, Resources: [Survivors]int{38, 38, 38, 38}, Hair: [Survivors]int{10, 11, 12, 9}},
	{Name: "acc07_adt_avt", Site: SiteHead, Resources: [Survivors]int{39, 39, 39, 39}, Hair: [Survivors]int{10, 31, 12, 9}},
	{Name: "acc08_adt_avt", Site: SiteHead, Resources: [Survivors]int{40, 40, 40, 40}, Hair: [Survivors]int{10, 31, 8, 9}},
	{Name: "acc09_adt_avt", Site: SiteHead, Resources: [Survivors]int{41, 41, 41, 41}, Hair: [Survivors]int{0, 0, 4, 0}},
	{Name: "acc20_adt_avt", Site: SiteHead, Resources: [Survivors]int{42, 42, 42, 42}, Hair: [Survivors]int{0, 11, 0, 0}},
	{Name: "acc21_adt_avt", Site: SiteHead, Resources: [Survivors]int{43, 43, 43, 43}, Hair: [Survivors]int{0, 0, 0, 9}},
	{Name: "acc22_adt_avt", Site: SiteHead, Resources: [Survivors]int{44, 44, 44, 44}, Hair: [Survivors]int{10, 0, 0, 0}},
	{Name: "acc23_adt_avt", Site: SiteHead, Resources: [Survivors]int{45, 45, 45, 45}, Hair: [Survivors]int{10, 0, 0, 0}},
	{Name: "acc24_adt_avt", Site: SiteHead, Resources: [Survivors]int{46, 46, 46, 46}, Hair: [Survivors]int{0, 0, 0, 9}},
	{Name: "acc25_adt_avt", Site: SiteHead, Resources: [Survivors]int{47, 47, 47, 47}, Hair: [Survivors]int{0, 11, 12, 0}},
	{Name: "acc26_adt_avt", Site: SiteHead, Resources: [Survivors]int{48, 48, 48, 48}, Hair: [Survivors]int{0, 0, 0, 0}},
	{Name: "acc27_adt_avt", Site: SiteHead, Resources: [Survivors]int{49, 49, 49, 49}, Hair: [Survivors]int{0, 0, 0, 0}},
	{Name: "acc28_adt_avt", Site: SiteHead, Resources: [Survivors]int{50, 50, 50, 50}, Hair: [Survivors]int{0, 0, 0, 0}},
	{Name: "acc29_adt_avt", Site: SiteHead, Resources: [Survivors]int{51, 51, 51, 51}, Hair: [Survivors]int{0, 0, 0, 0}},
	{Name: "amefuto_head_acc", Site: SiteHead, Resources: [Survivors]int{20, 20, 20, 20}, Hair: [Survivors]int{10, 11, 12, 9}},
	{Name: "baseball_head_acc", Site: SiteHead, Resources: [Survivors]int{21, 21, 21, 21}, Hair: [Survivors]int{6, 11, 8, 9}},
	{Name: "baseballcap_head_acc", Site: SiteHead, Resources: [Survivors]int{27, 27, 27, 27}, Hair: [Survivors]int{6, 31, 12, 9}},
	{Name: "catears_head_acc", Site: SiteHead, Resources: [Survivors]int{24, 24, 24, 24}, Hair: [Survivors]int{0, 0, 0, 0}},
	{Name: "firefighter_head_acc", Site: SiteHead, Resources: [Survivors]int{22, 22, 22, 22}, Hair: [Survivors]int{10, 11, 8, 9}},
	{Name: "hat_head_acc", Site: SiteHead, Resources: [Survivors]int{28, 28, 28, 28}, Hair: [Survivors]int{6, 31, 8, 9}},
	{Name: "headphone_head_acc", Site: SiteHead, Resources: [Survivors]int{26, 26, 26, 26}, Hair: [Survivors]int{0, 0, 0, 0}},
	{Name: "maid_head_acc", Site: SiteHead, Resources: [Survivors]int{25, 25, 25, 25}, Hair: [Survivors]int{0, 0, 0, 0}},
	{Name: "military_head_acc", Site: SiteHead, Resources: [Survivors]int{23, 23, 23, 23}, Hair: [Survivors]int{6, 11, 8, 9}},
	{Name: "santahat_head_acc", Site: SiteHead, Resources: [Survivors]int{30, 30, 30, 30}, Hair: [Survivors]int{10, 11, 12, 9}},
	{Name: "tengallon_head_acc", Site: SiteHead, Resources: [Survivors]int{29, 29, 29, 29}, Hair: [Survivors]int{6, 31, 8, 9}},
	{Name: "adt00_upper_avt", Site: SiteUpper, Resources: [Survivors]int{41, 51, 61, 71}},
	{Name: "adt01_upper_avt", Site: SiteUpper, Resources: [Survivors]int{42, 52, 62, 72}},
	{Name: "adt02_upper_avt", Site: SiteUpper, Resources: [Survivors]int{43, 53, 63, 73}},
	{Name: "adt03_upper_avt", Site: SiteUpper, Resources: [Survivors]int{44, 54, 64, 74}},
	{Name: "adt04_upper_avt", Site: SiteUpper, Resources: [Survivors]int{45, 55, 65, 75}},
	{Name: "adt05_upper_avt", Site: SiteUpper, Resources: [Survivors]int{46, 56, 66, 76}},
	{Name: "adt06_upper_avt", Site: SiteUpper, Resources: [Survivors]int{47, 57, 67, 77}},
	{Name: "adt07_upper_avt", Site: SiteUpper, Resources: [Survivors]int{48, 58, 68, 78}},
	{Name: "adt08_upper_avt", Site: SiteUpper, Resources: [Survivors]int{49, 59, 69, 79}},
	{Name: "adt09_upper_avt", Site: SiteUpper, Resources: [Survivors]int{50, 60, 70, 80}},
	{Name: "bobrun_upper_avt_f", Site: SiteUpper, Resources: [Survivors]int{0, 34, 34, 0}},
	{Name: "bobrun_upper_avt_m", Site: SiteUpper, Resources: [Survivors]int{33, 0, 0, 33}},
	{Name: "butler_upper_avt", Site: SiteUpper, Resources: [Survivors]int{23, 0, 0, 24}},
	{Name: "cowgirl_upper_avt", Site: SiteUpper, Resources: [Survivors]int{0, 25, 26, 0}},
	{Name: "doctor_upper_avt", Site: SiteUpper, Resources: [Survivors]int{21, 0, 0, 22}},
	{Name: "firefighter_upper_avt", Site: SiteUpper, Resources: [Survivors]int{1, 2, 3, 4}},
	{Name: "happi_upper_avt", Site: SiteUpper, Resources: [Survivors]int{37, 38, 39, 40}},
	{Name: "jersey_upper_avt", Site: SiteUpper, Resources: [Survivors]int{29, 0, 0, 0}},
	{Name: "military_upper_avt", Site: SiteUpper, Resources: [Survivors]int{5, 6, 7, 8}},
	{Name: "parka_upper_avt", Site: SiteUpper, Resources: [Survivors]int{13, 14, 15, 16}},
	{Name: "pcloth_upper_avt", Site: SiteUpper, Resources: [Survivors]int{0, 32, 31, 0}},
	{Name: "sailor_upper_avt", Site: SiteUpper, Resources: [Survivors]int{0, 0, 30, 0}},
	{Name: "swimwear_upper_avt", Site: SiteUpper, Resources: [Survivors]int{0, 35, 36, 0}},
	{Name: "tanktop_upper_avt", Site: SiteUpper, Resources: [Survivors]int{0, 27, 28, 0}},
	{Name: "tshirt_upper_avt", Site: SiteUpper, Resources: [Survivors]int{9, 10, 11, 12}},
	{Name: "tuxedo_upper_avt", Site: SiteUpper, Resources: [Survivors]int{17, 18, 19, 20}},
	{Name: "adt00_lower_avt", Site: SiteLower, Resources: [Survivors]int{33, 43, 53, 63}},
	{Name: "adt01_lower_avt", Site: SiteLower, Resources: [Survivors]int{34, 44, 54, 64}},
	{Name: "adt02_lower_avt", Site: SiteLower, Resources: [Survivors]int{35, 45, 55, 65}},
	{Name: "adt03_lower_avt", Site: SiteLower, Resources: [Survivors]int{36, 46, 56, 66}},
	{Name: "adt04_lower_avt", Site: SiteLower, Resources: [Survivors]int{37, 47, 57, 67}},
	{Name: "adt05_lower_avt", Site: SiteLower, Resources: [Survivors]int{38, 48, 58, 68}},
	{Name: "adt06_lower_avt", Site: SiteLower, Resources: [Survivors]int{39, 49, 59, 69}},
	{Name: "adt07_lower_avt", Site: SiteLower, Resources: [Survivors]int{40, 50, 60, 70}},
	{Name: "adt08_lower_avt", Site: SiteLower, Resources: [Survivors]int{41, 51, 61, 71}},
	{Name: "adt09_lower_avt", Site: SiteLower, Resources: [Survivors]int{42, 52, 62, 72}},
	{Name: "amefuto_lower_avt", Site: SiteLower, Resources: [Survivors]int{1, 2, 3, 4}},
	{Name: "bobrun_lower_avt_f", Site: SiteLower, Resources: [Survivors]int{0, 26, 26, 0}},
	{Name: "bobrun_lower_avt_m", Site: SiteLower, Resources: [Survivors]int{25, 0, 0, 25}},
	{Name: "butler_lower_avt", Site: SiteLower, Resources: [Survivors]int{17, 0, 0, 18}},
	{Name: "cowgirl_lower_avt", Site: SiteLower, Resources: [Survivors]int{0, 19, 20, 0}},
	{Name: "firefighter_lower_avt", Site: SiteLower, Resources: [Survivors]int{5, 6, 7, 8}},
	{Name: "happi_lower_avt", Site: SiteLower, Resources: [Survivors]int{29, 30, 31, 32}},
	{Name: "jersey_lower_avt", Site: SiteLower, Resources: [Survivors]int{21, 0, 0, 0}},
	{Name: "military_lower_avt", Site: SiteLower, Resources: [Survivors]int{9, 10, 11, 12}},
	{Name: "pcloth_lower_avt", Site: SiteLower, Resources: [Survivors]int{0, 24, 23, 0}},
	{Name: "sailor_lower_avt", Site: SiteLower, Resources: [Survivors]int{0, 0, 22, 0}},
	{Name: "swimwear_lower_avt", Site: SiteLower, Resources: [Survivors]int{0, 27, 28, 0}},
	{Name: "tuxedo_lower_avt", Site: SiteLower, Resources: [Survivors]int{13, 14, 15, 16}},
	{Name: "doctor_face_acc", Site: SiteFace, Resources: [Survivors]int{1, 1, 1, 1}},
	{Name: "eyepatch_face_eqp", Site: SiteFace, Resources: [Survivors]int{7, 8, 9, 10}},
	{Name: "gasmask_face_eqp", Site: SiteFace, Resources: [Survivors]int{3, 4, 5, 6}},
	{Name: "jason_face_eqp", Site: SiteFace, Resources: [Survivors]int{11, 12, 13, 14}},
	{Name: "santamoustache_face_eqp", Site: SiteFace, Resources: [Survivors]int{15, 16, 17, 18}},
	{Name: "sunglasses_face_acc", Site: SiteFace, Resources: [Survivors]int{2, 2, 2, 2}},
	{Name: "acc10_adt_avt", Site: SiteAccUpper, Resources: [Survivors]int{69, 69, 69, 69}},
	{Name: "acc11_adt_avt", Site: SiteAccUpper, Resources: [Survivors]int{39, 39, 39, 39}},
	{Name: "acc12_adt_avt", Site: SiteAccUpper, Resources: [Survivors]int{40, 40, 40, 40}},
	{Name: "acc13_adt_avt", Site: SiteAccUpper, Resources: [Survivors]int{41, 41, 41, 41}},
	{Name: "acc14_adt_avt", Site: SiteAccUpper, Resources: [Survivors]int{42, 42, 42, 42}},
	{Name: "amefuto_upper_eqp", Site: SiteAccUpper, Resources: [Survivors]int{19, 20, 21, 22}},
	{Name: "baseball_upper_eqp", Site: SiteAccUpper, Resources: [Survivors]int{23, 24, 25, 26}},
	{Name: "military_upper_eqp", Site: SiteAccUpper, Resources: [Survivors]int{27, 28, 29, 30}},
	{Name: "acc15_adt_avt", Site: SiteAccLower, Resources: [Survivors]int{43, 43, 43, 43}},
	{Name: "acc16_adt_avt", Site: SiteAccLower, Resources: [Survivors]int{44, 44, 44, 44}},
	{Name: "acc17_adt_avt", Site: SiteAccLower, Resources: [Survivors]int{45, 45, 45, 45}},
	{Name: "acc18_adt_avt", Site: SiteAccLower, Resources: [Survivors]int{46, 46, 46, 46}},
	{Name: "acc19_adt_avt", Site: SiteAccLower, Resources: [Survivors]int{47, 47, 47, 47}},
	{Name: "baseball_lower_eqp", Site: SiteAccLower, Resources: [Survivors]int{31, 32, 33, 34}},
	{Name: "military_lower_eqp", Site: SiteAccLower, Resources: [Survivors]int{35, 36, 37, 38}},
}
