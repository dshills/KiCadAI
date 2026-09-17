package boardfamily

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// These are deliberately checks for the pinned KiCad export dialect, not a
// general Gerber geometry parser or a general CNC/Excellon interpreter.
func verifyGerberFraming(s, function string) error {
	function = strings.Replace(function, "SolderPaste,", "Paste,", 1)
	function = strings.Replace(function, "SolderMask,", "Soldermask,", 1)
	polarity := "Positive"
	if strings.HasPrefix(function, "Soldermask,") {
		polarity = "Negative"
	}
	if function == "Profile" {
		function, polarity = "Profile,NP", ""
	}
	for prefix, expected := range map[string]string{
		"%TF.FileFunction,": "%TF.FileFunction," + function + "*%",
		"%MO":               "%MOMM*%",
		"%FS":               "%FSLAX46Y46*%",
	} {
		if strings.Count(s, prefix) != 1 || !hasGerberLine(s, expected) {
			return fmt.Errorf("missing, conflicting or unsupported Gerber declaration: %s", prefix)
		}
	}
	if polarity == "" {
		if strings.Contains(s, "%TF.FilePolarity,") {
			return errors.New("unexpected Gerber profile polarity")
		}
	} else if strings.Count(s, "%TF.FilePolarity,") != 1 || !hasGerberLine(s, "%TF.FilePolarity,"+polarity+"*%") {
		return errors.New("missing/conflicting Gerber file polarity")
	}
	if strings.Count(s, "M02*") != 1 || !strings.HasSuffix(strings.TrimSpace(s), "\nM02*") {
		return errors.New("missing, early or duplicate Gerber terminator")
	}
	// Reject legacy commands that would override the checked format/units or
	// terminate processing before the checked file end. Geometry is independently
	// reviewed from the actual native exports; it is not certified by this check.
	for _, token := range strings.Split(s, "*") {
		token = strings.Trim(token, " \t\r\n%")
		for _, prefix := range []string{"G70", "G71", "G90", "G91", "G070", "G071", "G090", "G091", "M00", "M01", "M0", "M1", "M2"} {
			if strings.HasPrefix(token, prefix) && token != "M02" {
				return errors.New("unsupported Gerber modal/stop command")
			}
		}
	}
	return nil
}

func hasGerberLine(s, line string) bool {
	return strings.Contains("\n"+s, "\n"+line+"\n")
}

var drillTool = regexp.MustCompile(`^T([1-9][0-9]*)C([0-9]+(?:\.[0-9]+)?)$`)
var drillSelect = regexp.MustCompile(`^T[1-9][0-9]*$`)
var drillPoint = regexp.MustCompile(`^X(-?[0-9]+(?:\.[0-9]+)?)Y(-?[0-9]+(?:\.[0-9]+)?)$`)

// parseKiCadDrills accepts only the absolute metric decimal drill-only output
// requested by exportManufacturing. Unknown commands are errors, never comments.
func parseKiCadDrills(s, kind string) ([]drillHit, error) {
	plating := "; #@! TF.FileFunction,Plated,1,2,PTH"
	if kind == "NPTH" {
		plating = "; #@! TF.FileFunction,NonPlated,1,2,NPTH"
	} else if kind != "PTH" {
		return nil, errors.New("unknown drill plating kind")
	}
	state := "start"
	metric, format, identity := false, false, false
	tools := map[string]float64{}
	selected := ""
	var got []drillHit
	for i, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		accepted := false
		switch state {
		case "start":
			if line == "M48" {
				state, accepted = "header", true
			}
		case "header":
			switch {
			case strings.HasPrefix(line, "; #@! TF.FileFunction,"):
				if !identity && line == plating {
					identity, accepted = true, true
				}
			case strings.HasPrefix(line, ";"):
				accepted = true
			case line == "FMAT,2" && !format && !metric:
				format, accepted = true, true
			case line == "METRIC" && format && !metric:
				metric, accepted = true, true
			case line == "%" && metric && identity:
				state, accepted = "absolute", true
			default:
				if m := drillTool.FindStringSubmatch(line); m != nil && metric {
					diameter, err := strconv.ParseFloat(m[2], 64)
					name := "T" + m[1]
					if _, exists := tools[name]; !exists && err == nil && diameter > 0 && !math.IsInf(diameter, 0) {
						tools[name], accepted = diameter, true
					}
				}
			}
		case "absolute":
			if line == "G90" {
				state, accepted = "drill", true
			}
		case "drill":
			if line == "G05" {
				state, accepted = "body", true
			}
		case "body":
			switch {
			case line == "M30":
				state, accepted = "end", true
			case drillSelect.MatchString(line):
				if _, exists := tools[line]; exists {
					selected, accepted = line, true
				}
			default:
				if m := drillPoint.FindStringSubmatch(line); m != nil && selected != "" {
					x, ex := strconv.ParseFloat(m[1], 64)
					y, ey := strconv.ParseFloat(m[2], 64)
					if ex == nil && ey == nil && !math.IsInf(x, 0) && !math.IsInf(y, 0) {
						got = append(got, drillHit{x, y, tools[selected]})
						accepted = true
					}
				}
			}
		}
		if !accepted {
			return nil, fmt.Errorf("unsupported or misplaced Excellon command at line %d (%s)", i+1, state)
		}
	}
	if state != "end" {
		return nil, errors.New("incomplete Excellon file")
	}
	return got, nil
}
