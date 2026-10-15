package theme

// Palette definitions for every built-in theme. Empty fields keep the
// library default.

var (
	oceanLight = Palette{
		Bg: "#EAF1F5", Surface: "#F6FAFC", SurfaceHover: "#DCE8EF",
		Text: "#16232B", TextMuted: "#526772", Border: "#B9CDD8", ControlBorder: "#6E8592",
		Accent: "#0F5C7A", AccentHover: "#0B4A61", AccentPressed: "#083A4C",
		OnAccent: "#F2FAFF", Selection: "#CFE3EC", Focus: "#0F5C7A",
		AccentText: "#0F5C7A", Ornament: "#3C8FB0",
		Success: "#1F6B4A", Warning: "#8A5A12", Danger: "#A12635", Info: "#24608A", DangerFill: "#A12635", OnDanger: "#F2FAFF",
	}
	oceanDark = Palette{
		Bg: "#0F1A20", Surface: "#16252D", SurfaceHover: "#1F333D",
		Text: "#E8F2F7", TextMuted: "#A6BCC6", Border: "#2F4A57", ControlBorder: "#7C97A3",
		Accent: "#2E93BC", AccentHover: "#3BA3CD", AccentPressed: "#2279A0",
		OnAccent: "#F2FAFF", Selection: "#1E3B49", Focus: "#7FC4E0",
		AccentText: "#7FC4E0", Ornament: "#5AA9C9",
		Success: "#8FCBAE", Warning: "#DCC07E", Danger: "#EF9AA5", Info: "#9CC2E0", DangerFill: "#A12635", OnDanger: "#F2FAFF",
	}

	forestLight = Palette{
		Bg: "#EDF1E7", Surface: "#F8FAF3", SurfaceHover: "#DDE6D2",
		Text: "#1C2418", TextMuted: "#5A6852", Border: "#C2CFB4", ControlBorder: "#788A6A",
		Accent: "#2F6B3A", AccentHover: "#25562F", AccentPressed: "#1D4425",
		OnAccent: "#F7FBF2", Selection: "#D6E3C9", Focus: "#2F6B3A",
		AccentText: "#2F6B3A", Ornament: "#7E9B5B",
		Success: "#2F6B3A", Warning: "#8A5A12", Danger: "#A12635", Info: "#355E80", DangerFill: "#A12635", OnDanger: "#F7FBF2",
	}
	forestDark = Palette{
		Bg: "#141A12", Surface: "#1E271A", SurfaceHover: "#2B3725",
		Text: "#EAF0E2", TextMuted: "#AEBEA2", Border: "#3E4E34", ControlBorder: "#87997A",
		Accent: "#5EA96C", AccentHover: "#6FBA7D", AccentPressed: "#4B8F58",
		OnAccent: "#F7FBF2", Selection: "#2B3D28", Focus: "#9ED0A6",
		AccentText: "#9ED0A6", Ornament: "#8FAE6A",
		Success: "#94C6A4", Warning: "#DFC080", Danger: "#F0A1AA", Info: "#9DBDDA", DangerFill: "#A12635", OnDanger: "#F7FBF2",
	}

	graphiteLight = Palette{
		Bg: "#F0F0F2", Surface: "#FAFAFB", SurfaceHover: "#E2E2E6",
		Text: "#1A1A1D", TextMuted: "#5C5C63", Border: "#C8C8CF", ControlBorder: "#7A7A83",
		Accent: "#4A4A55", AccentHover: "#3A3A44", AccentPressed: "#2C2C34",
		OnAccent: "#F7F7F9", Selection: "#DCDCE2", Focus: "#4A4A55",
		AccentText: "#3A3A44", Ornament: "#8A8A96",
		Success: "#326247", Warning: "#805714", Danger: "#A12635", Info: "#355E80", DangerFill: "#A12635", OnDanger: "#F7F7F9",
	}
	graphiteDark = Palette{
		Bg: "#171719", Surface: "#212124", SurfaceHover: "#2E2E33",
		Text: "#EDEDF0", TextMuted: "#B0B0B8", Border: "#45454C", ControlBorder: "#8B8B94",
		Accent: "#9A9AA6", AccentHover: "#AAAAB6", AccentPressed: "#84848F",
		OnAccent: "#171719", Selection: "#33333A", Focus: "#C9C9D2",
		AccentText: "#C9C9D2", Ornament: "#A5A5B0",
		Success: "#94C6A4", Warning: "#DFC080", Danger: "#F0A1AA", Info: "#9DBDDA", DangerFill: "#A12635", OnDanger: "#F7F7F9",
	}

	sunsetLight = Palette{
		Bg: "#F7EEE6", Surface: "#FDF7F1", SurfaceHover: "#F0DFD0",
		Text: "#2A1D17", TextMuted: "#6E574A", Border: "#DCC7B3", ControlBorder: "#93796A",
		Accent: "#B5541F", AccentHover: "#9A4516", AccentPressed: "#7E3711",
		OnAccent: "#FFF7F0", Selection: "#F0D9C8", Focus: "#B5541F",
		AccentText: "#B5541F", Ornament: "#C98A3F",
		Success: "#326247", Warning: "#95610E", Danger: "#A12635", Info: "#355E80", DangerFill: "#A12635", OnDanger: "#FFF7F0",
	}
	sunsetDark = Palette{
		Bg: "#1E1512", Surface: "#2A1E19", SurfaceHover: "#3B2A22",
		Text: "#F5E9E0", TextMuted: "#C4AB9C", Border: "#543C30", ControlBorder: "#A08574",
		Accent: "#E1793B", AccentHover: "#EC8A4E", AccentPressed: "#C4632C",
		OnAccent: "#2A1D17", Selection: "#4A2E20", Focus: "#F0A96E",
		AccentText: "#F0A96E", Ornament: "#D9A25B",
		Success: "#94C6A4", Warning: "#DFC080", Danger: "#F0A1AA", Info: "#9DBDDA", DangerFill: "#A12635", OnDanger: "#FFF7F0",
	}

	roseLight = Palette{
		Bg: "#F9ECEF", Surface: "#FDF6F8", SurfaceHover: "#F2DCE2",
		Text: "#2C1A1E", TextMuted: "#705761", Border: "#DFC2CB", ControlBorder: "#96737F",
		Accent: "#B0284E", AccentHover: "#962041", AccentPressed: "#7C1834",
		OnAccent: "#FFF4F7", Selection: "#F1D3DC", Focus: "#B0284E",
		AccentText: "#B0284E", Ornament: "#C86B84",
		Success: "#326247", Warning: "#95610E", Danger: "#A12635", Info: "#355E80", DangerFill: "#A12635", OnDanger: "#FFF4F7",
	}
	roseDark = Palette{
		Bg: "#1F1317", Surface: "#2C1B21", SurfaceHover: "#3E272F",
		Text: "#F6E7EC", TextMuted: "#C9A9B4", Border: "#553540", ControlBorder: "#A5808C",
		Accent: "#E0608A", AccentHover: "#EA7298", AccentPressed: "#C64D75",
		OnAccent: "#2C1A1E", Selection: "#4B2933", Focus: "#F0A0BB",
		AccentText: "#F0A0BB", Ornament: "#D98BA3",
		Success: "#94C6A4", Warning: "#DFC080", Danger: "#F0A1AA", Info: "#9DBDDA", DangerFill: "#A12635", OnDanger: "#FFF4F7",
	}

	lavenderLight = Palette{
		Bg: "#F0EEF7", Surface: "#F8F7FC", SurfaceHover: "#E4E0F1",
		Text: "#201B2C", TextMuted: "#605A72", Border: "#CCC6DE", ControlBorder: "#8079A0",
		Accent: "#5B45A8", AccentHover: "#4C3990", AccentPressed: "#3F2F79",
		OnAccent: "#F7F5FF", Selection: "#DDD6EE", Focus: "#5B45A8",
		AccentText: "#5B45A8", Ornament: "#8C7BC0",
		Success: "#326247", Warning: "#95610E", Danger: "#A12635", Info: "#355E80", DangerFill: "#A12635", OnDanger: "#F7F5FF",
	}
	lavenderDark = Palette{
		Bg: "#161320", Surface: "#211C2E", SurfaceHover: "#302941",
		Text: "#EBE7F5", TextMuted: "#B2AAC8", Border: "#443A5B", ControlBorder: "#8E84AC",
		Accent: "#A98FE8", AccentHover: "#B7A1EE", AccentPressed: "#9176D8",
		OnAccent: "#201B2C", Selection: "#372E4C", Focus: "#C6B4F4",
		AccentText: "#C6B4F4", Ornament: "#A392D6",
		Success: "#94C6A4", Warning: "#DFC080", Danger: "#F0A1AA", Info: "#9DBDDA", DangerFill: "#A12635", OnDanger: "#F7F5FF",
	}

	mintLight = Palette{
		Bg: "#E9F4EE", Surface: "#F5FBF7", SurfaceHover: "#D8EBE0",
		Text: "#15251D", TextMuted: "#4E6A5B", Border: "#B8D3C4", ControlBorder: "#6E8D7C",
		Accent: "#177A52", AccentHover: "#126545", AccentPressed: "#0E5238",
		OnAccent: "#F2FCF7", Selection: "#CCE7D9", Focus: "#177A52",
		AccentText: "#177A52", Ornament: "#4CA47F",
		Success: "#177A52", Warning: "#8A5A12", Danger: "#A12635", Info: "#24608A", DangerFill: "#A12635", OnDanger: "#F2FCF7",
	}
	mintDark = Palette{
		Bg: "#0F1A15", Surface: "#16261E", SurfaceHover: "#1F362B",
		Text: "#E6F3EC", TextMuted: "#A0C0B0", Border: "#2F4E3E", ControlBorder: "#7C9C8B",
		Accent: "#35B37C", AccentHover: "#45C08A", AccentPressed: "#2A9668",
		OnAccent: "#0F1A15", Selection: "#1E3D2F", Focus: "#7EDCB0",
		AccentText: "#7EDCB0", Ornament: "#57B78C",
		Success: "#8FCBAE", Warning: "#DFC080", Danger: "#EF9AA5", Info: "#9CC2E0", DangerFill: "#A12635", OnDanger: "#F2FCF7",
	}

	amberLight = Palette{
		Bg: "#FAF1E0", Surface: "#FDF9F0", SurfaceHover: "#F3E2C4",
		Text: "#2B2110", TextMuted: "#6E5C3C", Border: "#DBC6A0", ControlBorder: "#947C56",
		Accent: "#A96A0B", AccentHover: "#8F5908", AccentPressed: "#754906",
		OnAccent: "#FFF9EE", Selection: "#F0DCB8", Focus: "#A96A0B",
		AccentText: "#A96A0B", Ornament: "#C6923B",
		Success: "#326247", Warning: "#A96A0B", Danger: "#A12635", Info: "#355E80", DangerFill: "#A12635", OnDanger: "#FFF9EE",
	}
	amberDark = Palette{
		Bg: "#1D160B", Surface: "#292012", SurfaceHover: "#3A2E1B",
		Text: "#F5ECDC", TextMuted: "#C8B18C", Border: "#513F26", ControlBorder: "#9C8360",
		Accent: "#DFA644", AccentHover: "#E8B257", AccentPressed: "#C78F33",
		OnAccent: "#1D160B", Selection: "#453419", Focus: "#F0C877",
		AccentText: "#F0C877", Ornament: "#D4A554",
		Success: "#94C6A4", Warning: "#DFC080", Danger: "#F0A1AA", Info: "#9DBDDA", DangerFill: "#A12635", OnDanger: "#FFF9EE",
	}

	slateLight = Palette{
		Bg: "#ECF0F2", Surface: "#F7F9FA", SurfaceHover: "#DDE4E8",
		Text: "#1B2326", TextMuted: "#566469", Border: "#C2CCD1", ControlBorder: "#77868C",
		Accent: "#2C5C6E", AccentHover: "#234D5C", AccentPressed: "#1B3F4C",
		OnAccent: "#F2F9FB", Selection: "#D2E0E5", Focus: "#2C5C6E",
		AccentText: "#2C5C6E", Ornament: "#5E8A99",
		Success: "#2F6B4A", Warning: "#8A5A12", Danger: "#A12635", Info: "#2C5C6E", DangerFill: "#A12635", OnDanger: "#F2F9FB",
	}
	slateDark = Palette{
		Bg: "#121819", Surface: "#1B2426", SurfaceHover: "#263235",
		Text: "#E6EEF0", TextMuted: "#A2B4BA", Border: "#324347", ControlBorder: "#7E9298",
		Accent: "#4E93A8", AccentHover: "#5DA3B8", AccentPressed: "#3F7E92",
		OnAccent: "#121819", Selection: "#22363C", Focus: "#8FC6D6",
		AccentText: "#8FC6D6", Ornament: "#6CA4B4",
		Success: "#94C6A4", Warning: "#DFC080", Danger: "#F0A1AA", Info: "#8FC6D6", DangerFill: "#A12635", OnDanger: "#F2F9FB",
	}

	monoLight = Palette{
		Bg: "#FFFFFF", Surface: "#F5F5F5", SurfaceHover: "#E8E8E8",
		Text: "#101010", TextMuted: "#5A5A5A", Border: "#CACACA", ControlBorder: "#707070",
		Accent: "#1A1A1A", AccentHover: "#000000", AccentPressed: "#000000",
		OnAccent: "#FFFFFF", Selection: "#E0E0E0", Focus: "#1A1A1A",
		AccentText: "#1A1A1A", Ornament: "#7A7A7A",
		Success: "#256B45", Warning: "#7A5210", Danger: "#A12635", Info: "#2C5C6E", DangerFill: "#A12635", OnDanger: "#FFFFFF",
	}
	monoDark = Palette{
		Bg: "#0E0E0E", Surface: "#1A1A1A", SurfaceHover: "#282828",
		Text: "#F2F2F2", TextMuted: "#ADADAD", Border: "#3C3C3C", ControlBorder: "#8A8A8A",
		Accent: "#E8E8E8", AccentHover: "#FFFFFF", AccentPressed: "#CACACA",
		OnAccent: "#0E0E0E", Selection: "#333333", Focus: "#FFFFFF",
		AccentText: "#E8E8E8", Ornament: "#9A9A9A",
		Success: "#94C6A4", Warning: "#DFC080", Danger: "#F0A1AA", Info: "#9CC2E0", DangerFill: "#A12635", OnDanger: "#0E0E0E",
	}
)
