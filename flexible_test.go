package main

import (
 "os"
 "strings"
 "testing"
)

func TestAlternateFivePageDrawingLayout(t *testing.T){
 b,e:=os.ReadFile("testdata/SYNTHETIC_ALTERNATE_LAYOUT.pdf");if e!=nil {t.Fatal(e)}
 a,e:=AnalyzeDocument(b);if e!=nil {t.Fatal(e)}
 if a.Pages!=5 {t.Fatalf("pages: %d",a.Pages)}
 if len(a.Wires)<4 {t.Fatalf("alternate conductor recognition returned %d rows",len(a.Wires))}
 if len(a.BOM)<2 {t.Fatalf("alternate BOM recognition returned %d rows",len(a.BOM))}
 found:=false
 for _,c:=range a.Connectors {if strings.Contains(c.Part,"D38999/26FG35SA") {found=true}}
 if !found {t.Fatalf("alternative connector part not found: %+v",a.Connectors)}
}

func TestToolingUIHasSimpleViewAndExit(t *testing.T){
 b,e:=os.ReadFile("assets/ui.html");if e!=nil {t.Fatal(e)}
 s:=string(b)
 for _,phrase:=range []string{"toolingOverview","quickToolLookup","Show detailed manufacturer tables","Exit application","/api/exit"} {
   if !strings.Contains(s,phrase) {t.Errorf("UI regression missing %s",phrase)}
 }
}
