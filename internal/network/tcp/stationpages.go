package tcp

import (
	"fmt"
	"strings"
)

// STATION's account pages trimmed to what this server needs (the owner,
// 2026-11-15: "change the station page to faciliate the email verification and
// remove the old fields that create account doesnt need, no credit card address
// etc"). With mail.accounts on, repointStationPak serves these in place of the
// pak's own IDCreate, IDCreat2, UpAcct, FrgtPass and AcctMgmt pages: a PS2
// reads STATION's pages from the card's station.pak, which the patcher writes
// (vanilla reads its disc's).
//
// STATION's code reads every field of the old forms by name and refuses a form
// that lacks one ("Error, invalid page contents"; ContinueAcctCreate,
// SubmitAcctCreate, SubmitAcctUpdate, SubmitForgotPassword in
// decomp/modules/station/src/viwndgenericrenderer.cpp), and CheckName wants a
// letter or a digit in each it checks. So a field no player fills in is kept as
// a hidden one: a Text of that name holding a value that passes, with no grid
// position (it is never selected) and placed before the page's two
// backgrounds, which are drawn after it over the whole screen (VIGUIPage::Draw
// draws in the order the page lists). Nothing the server keeps comes of them:
// handleCreateAccount reads the name, the password and the e-mail alone.
//
// What a player sees:
//   - IDCreate: User Name, Password, Re-enter Password, Continue.
//   - IDCreat2: E-mail Address (required: the server refuses an account
//     without one, and says so), Create Account. The account is made with it
//     and its verify link mailed at once.
//   - AcctMgmt: Change Password, Change E-mail (no billing, no cancelling).
//   - UpAcct: E-mail Address, Update (the new address mailed its link).
//   - FrgtPass: User Name, E-mail Address, Submit (a reset link to a verified
//     address that matches).
//
// STATION's own check of an address (CheckEmail) takes letters, digits, '_',
// '@' and '.' alone; an address with a '-' or a '+' is set on the website.

// stationHidden is a field the code needs and no player fills in.
func stationHidden(name, val string) string {
	return fmt.Sprintf("\t<Text name=%q val=%q>\n\t\t<Position x=\"0\" y=\"0\"/>\n\t\t<Size width=\"24\" height=\"16\"/>\n"+
		"\t\t<TextColor r=\"0\" g=\"0\" b=\"0\"/>\n\t\t<Shadow active=\"false\"/>\n\t</Text>\n", name, val)
}

// stationBackdrop is the pages' two backgrounds and the help panel's shade.
const stationBackdrop = `	<Object>
		<Position x="0" y="0"/>
		<Image imageName="P:\studio\eqps2\Game_Assets\UI\stbk00.tga"/>
	</Object>
	<Object>
		<Position x="320" y="0"/>
		<Image imageName="P:\studio\eqps2\Game_Assets\UI\stbk10.tga"/>
	</Object>
	<Object>
		<Position x="440" y="87"/>
		<Size width="190" height="282"/>
		<BGColor r="0" g="0" b="0" a="128"/>
	</Object>
`

// stationLabel is a field's label at y.
func stationLabel(name, val string, x, y int) string {
	n := ""
	if name != "" {
		n = fmt.Sprintf(" name=%q", name)
	}
	return fmt.Sprintf("\t<Text%s val=%q>\n\t\t<Position x=\"%d\" y=\"%d\"/>\n\t\t<TextColor r=\"255\" g=\"226\" b=\"97\"/>\n\t</Text>\n",
		n, val, x, y+3)
}

// stationField is a text field at grid row row and y; password stars it.
func stationField(name, title string, row, y, maxChars int, password bool) string {
	pw := ""
	if password {
		pw = "\t\t<Password mode=\"true\"/>\n"
	}
	return fmt.Sprintf("\t<Text name=%q val=\"\">\n\t\t<ButtonPos x=\"0\" y=\"%d\"/>\n\t\t<Position x=\"230\" y=\"%d\"/>\n"+
		"\t\t<Size width=\"162\" height=\"25\"/>\n\t\t<TextColor r=\"0\" g=\"0\" b=\"0\"/>\n\t\t<RollColor r=\"20\" g=\"20\" b=\"200\"/>\n"+
		"\t\t<Image tile=\"horiz\" capTexture=\"TextCapL\" imageType=\"TextTile\"/>\n\t\t<ImageOffset x=\"-10\"/>\n"+
		"\t\t<SelectEffect type=\"scrollleft\" xtarget=\"216\"/>\n\t\t<Shadow active=\"false\"/>\n%s"+
		"\t\t<MaxChars size=\"%d\"/>\n\t\t<EditLabel title=%q/>\n\t</Text>\n", name, row, y, pw, maxChars, title)
}

// stationButton is a button at grid row row; to is its action, or a page
// ("data/ui/...") it opens.
func stationButton(val, to string, row, x, y, width int) string {
	act := fmt.Sprintf("action=%q", to)
	if strings.HasPrefix(to, "data/") {
		act = fmt.Sprintf("nextPage=%q", to)
	}
	return fmt.Sprintf("\t<Button val=%q alignment=\"center\" %s>\n\t\t<ButtonPos x=\"0\" y=\"%d\"/>\n"+
		"\t\t<Position x=\"%d\" y=\"%d\"/>\n\t\t<TextColor r=\"255\" g=\"226\" b=\"97\"/>\n\t\t<RollColor r=\"255\" g=\"226\" b=\"97\"/>\n"+
		"\t\t<Size width=\"%d\" height=\"28\"/>\n\t\t<Image alpha=\"128\" tile=\"horiz\" capTexture=\"ButtonCapL\" imageType=\"ButtonTile\"/>\n"+
		"\t\t<ImageOffset x=\"0\"/>\n\t\t<SelectEffect type=\"fadealpha\" alphatarget=\"255\"/>\n\t</Button>\n", val, act, row, x, y, width)
}

// stationHelp is the right-hand panel: its title and its text.
func stationHelp(title, text string) string {
	return fmt.Sprintf("\t<Text val=%q alignment=\"center\" >\n\t\t<Position x=\"440\" y=\"87\"/>\n\t\t<Size width=\"190\" height=\"32\"/>\n"+
		"\t\t<TextColor r=\"220\" g=\"220\" b=\"220\"/>\n\t</Text>\n"+
		"\t<Text val=%q>\n\t\t<Position x=\"450\" y=\"118\"/>\n\t\t<Size width=\"187\" height=\"250\"/>\n"+
		"\t\t<TextColor r=\"220\" g=\"220\" b=\"220\"/>\n\t\t<Wrap mode=\"true\"/>\n\t</Text>\n", title, text)
}

// stationFooter is the selector and the Back / Select hints.
func stationFooter(selectorX int) string {
	return fmt.Sprintf(`	<Selector>
		<Image imageType="BluePointerR"/>
		<SelectorPos x="%d"/>
		<ImageOffset y="5"/>
	</Selector>
	<Object>
		<Position x="237" y="380"/>
		<Image imageType="StationTri"/>
	</Object>
	<Text val="Back">
		<Position x="265" y="378"/>
		<TextColor r="255" g="226" b="97"/>
	</Text>
	<Object>
		<Position x="328" y="380"/>
		<Image imageType="StationX"/>
	</Object>
	<Text val="Select">
		<Position x="356" y="378"/>
		<TextColor r="255" g="226" b="97"/>
	</Text>
`, selectorX)
}

func stationPage(width, height int, body ...string) []byte {
	return []byte(fmt.Sprintf("<PS2GUIPage gridWidth=\"%d\" gridHeight=\"%d\">\n%s</PS2GUIPage>\n", width, height, strings.Join(body, "")))
}

// stationEmailPages are the pages served with mail.accounts on, by their pak
// names; page is the account website as a player types it.
func stationEmailPages(page string) map[string][]byte {
	pwMax := 32 // auth.MaxPasswordLen and its NUL (raisePasswordFields)
	return map[string][]byte{
		"data/ui/IDCreate.xml": stationPage(3, 4,
			stationHidden("FirstName", "x"), stationHidden("MiddleName", "x"), stationHidden("LastName", "x"),
			stationBackdrop,
			stationLabel("StationNameLabel", "User Name", 30, 140), stationField("StationName", "User Name", 0, 139, 16, false),
			stationLabel("Password1Label", "Password", 30, 177), stationField("StationPassword", "Password", 1, 176, pwMax, true),
			stationLabel("Password2Label", "Re-enter Password", 30, 214), stationField("StationPassword2", "Re-enter Password", 2, 213, pwMax, true),
			stationButton("Continue", "ContinueAcctCreate", 3, 140, 270, 162),
			stationFooter(10),
			stationHelp("Create Account", "Choose a user name and a password, and enter the password twice. Next you give your "+
				"e-mail address: we send a link to it to verify your account.")),
		"data/ui/IDCreat2.xml": stationPage(3, 2,
			stationHidden("Country", "x"), stationHidden("PostalCode", "x"), stationHidden("BirthMonth", "Jan"),
			stationHidden("BirthDay", "1"), stationHidden("BirthYear", "1977"), stationHidden("Gender", "x"),
			stationBackdrop,
			stationLabel("EmailAddressLabel", "E-mail Address", 30, 177), stationField("EmailAddress", "E-mail Address", 0, 176, 100, false),
			stationButton("Create Account", "SubmitAcctCreate", 1, 140, 270, 162),
			stationFooter(10),
			stationHelp("Your E-mail", "Your e-mail address is required. Once the account is made, check your e-mail: open the "+
				"link we send to verify your account. With it verified you can reset a forgotten password by e-mail. "+
				"An address with a - or + in it: make the account at "+page+" instead.")),
		"data/ui/AcctMgmt.xml": stationPage(1, 2,
			stationBackdrop,
			stationButton("Change Password", "data/ui/PassChng.xml", 0, 40, 170, 370),
			stationButton("Change E-mail", "data/ui/UpAcct.xml", 1, 40, 215, 370),
			stationFooter(10),
			stationHelp("Manage Account", "Change your password, or the e-mail address on your account. A password can also "+
				"be changed, and your e-mail checked, at "+page+".")),
		"data/ui/UpAcct.xml": stationPage(3, 2,
			stationHidden("Country", "x"), stationHidden("PostalCode", "x"),
			stationBackdrop,
			stationLabel("EmailAddressLabel", "E-mail Address", 30, 177), stationField("EmailAddress", "E-mail Address", 0, 176, 100, false),
			stationButton("Update", "SubmitAcctUpdate", 1, 140, 270, 162),
			stationFooter(10),
			stationHelp("Change E-mail", "Enter your new e-mail address. We send a link to it: your account is verified again "+
				"once you open it. An account with an authenticator changes it at "+page+".")),
		"data/ui/FrgtPass.xml": stationPage(3, 3,
			stationHidden("BirthMonth", "Jan"), stationHidden("BirthDay", "1"), stationHidden("BirthYear", "1977"),
			stationBackdrop,
			stationLabel("UsernameLabel", "User Name", 30, 140), stationField("Username", "User Name", 0, 139, 32, false),
			stationLabel("EmailAddressLabel", "E-mail Address", 30, 177), stationField("EmailAddress", "E-mail Address", 1, 176, 100, false),
			stationButton("Submit", "SubmitForgotPassword", 2, 140, 270, 162),
			stationFooter(10),
			stationHelp("Forgot Password", "Enter your user name and your account's e-mail address. If it is verified, we "+
				"e-mail you a link to choose a new password. No verified e-mail? Ask a GM at "+page+".")),
	}
}
