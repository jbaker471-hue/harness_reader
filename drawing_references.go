package main

import (
 "fmt"
 "regexp"
 "strings"
)

// Only explicit prose references are matched here. Bare numbers, circled
// callouts and note symbols require separate geometry-aware interpretation.
var explicitBOMReference = regexp.MustCompile(`(?i)\b(?:FIND|ITEM)(?:\s+(?:NO\.?|NUMBER))?\s*#?\s+([0-9]{1,4}[A-Z]?)(?:\s*[,;).:]|\s|$)`)
var unusedTreatmentNote = regexp.MustCompile(`(?i)\bUNUSED\b.*\b(?:CONTACT|CONTACTS|CAVITY|CAVITIES|OPENINGS)\b`)

func canonicalFind(s string) string {
 s=strings.ToUpper(strings.TrimSpace(s))
 s=strings.TrimLeft(s,"0")
 if s=="" {return "0"}
 return s
}

func drawingReferenceFindings(d PDFData, bom []BOMRow) []Finding {
 parts:=map[string]map[string]bool{}
 for _,b:=range bom {
  if strings.TrimSpace(b.Item)=="" {continue}
  key:=canonicalFind(b.Item)
  if parts[key]==nil {parts[key]=map[string]bool{}}
  parts[key][compactPN(b.Part)]=true
 }
 var out []Finding
 seen:=map[string]bool{}
 for _,p:=range d.Pages {
  for _,line:=range pageLines(p) {
   for _,m:=range explicitBOMReference.FindAllStringSubmatch(line.Text,-1) {
    key:=canonicalFind(m[1])
    id:=fmt.Sprintf("%d:%s",p.Number,key);if seen[id]{continue};seen[id]=true
    matches:=parts[key]
    code,message:="BOM_REFERENCE_REVIEW","Explicit item/find reference "+key+" matches a recognized BOM item; verify its applicability and source callout."
    if len(bom)==0 {code="BOM_REFERENCE_UNRESOLVED";message="Explicit item/find reference "+key+" cannot be reconciled because no BOM was recognized. This is not proof of a drawing error."
    }else if len(matches)==0 {code="BOM_REFERENCE_UNRESOLVED";message="Explicit item/find reference "+key+" has no matching recognized BOM item. Check missing BOM rows, continuation sheets or a drawing discrepancy."
    }else if len(matches)>1||matches[""] {code="BOM_REFERENCE_AMBIGUOUS";message="Explicit item/find reference "+key+" maps to conflicting or incomplete BOM part identities; no part is selected."}
    out=append(out,Finding{Severity:"REVIEW",Category:CatReview,Code:code,Message:message,Pages:[]int{p.Number}})
   }
   text:=strings.ToUpper(line.Text)
   if unusedTreatmentNote.MatchString(text)&&strings.Contains(text,"PLUG") {
    key:=fmt.Sprintf("fill:%d",p.Number);if seen[key]{continue};seen[key]=true
    message:="Unused-position treatment is specified in drawing text. Review the complete note and select the applicable contact/plug policy; no quantity or policy is inferred."
    if regexp.MustCompile(`\bOR\b`).MatchString(text) {message="Unused-position note contains an OR alternative. Do not automatically require both filler contacts and sealing plugs; resolve the applicable treatment from the complete drawing requirements."}
    out=append(out,Finding{Severity:"REVIEW",Category:CatReview,Code:"UNUSED_TREATMENT_REVIEW",Message:message,Pages:[]int{p.Number}})
   }
  }
 }
 return out
}
