package tui

import "strings"

type commandRegistry struct {
	specs []slashCommandSpec
}

func newCommandRegistry(specs []slashCommandSpec) commandRegistry {
	return commandRegistry{specs: append([]slashCommandSpec(nil), specs...)}
}

func browserOnlyCommandPresentation(spec slashCommandSpec) slashCommandSpec {
	switch spec.Name {
	case "model":
		spec.Usage = "/model"
		spec.Description = "Xem trạng thái Gemini Web và phiên Chrome WEB-only"
	case "config":
		spec.Usage = "/config"
		spec.Description = "Cấu hình Chrome và hồ sơ đăng nhập Gemini Web"
	}
	return spec
}

func (r commandRegistry) Visible() []slashCommandSpec {
	var out []slashCommandSpec
	for _, spec := range r.specs {
		if !spec.Hidden {
			out = append(out, browserOnlyCommandPresentation(spec))
		}
	}
	return out
}

func (r commandRegistry) Find(name string) (slashCommandSpec, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return slashCommandSpec{}, false
	}
	for _, spec := range r.specs {
		if spec.matches(name) {
			return browserOnlyCommandPresentation(spec), true
		}
	}
	return slashCommandSpec{}, false
}

func (r commandRegistry) PaletteItems() []commandPaletteItem {
	var items []commandPaletteItem
	for _, spec := range r.Visible() {
		items = append(items, commandPaletteItem{
			Name:        spec.Name,
			Aliases:     append([]string(nil), spec.Aliases...),
			Usage:       spec.Usage,
			Description: spec.Description,
			AutoExecute: spec.AutoExecute,
		})
	}
	return items
}
