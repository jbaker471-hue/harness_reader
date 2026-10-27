package main

import (
 "fmt"
 "strings"
 "testing"
)

func inventedPairedPage() Page {
 p:=Page{Number:1,Width:1000,Height:800,PageAccurate:true}
 add:=func(x,y float64,text string){p.Words=append(p.Words,Word{Text:text,X:x,Y:y,Size:8})}
 add(55,770,"RACK-J7");add(790,770,"BENCH-P9")
 for i,pin:=range []string{"A","a","3","4"} {
  y:=730-float64(i)*180
  add(55,y,pin);add(72,y,"WHT");add(420,y,"24");add(440,y,"AWG")
  add(765,y,"WH");add(790,y,fmt.Sprint(i+1));add(820,y,"SPARE")
 }
 return p
}

func TestPairedSchematicAcrossPage(t *testing.T) {
 p:=inventedPairedPage()
 // The lower two rows need their own repeated connector headings, as on
 // split schematic blocks; all positions here are invented.
 p.Words=append(p.Words,Word{Text:"RACK-J7",X:55,Y:410,Size:8},Word{Text:"BENCH-P9",X:790,Y:410,Size:8})
 rows:=pairedSchematicRows(p)
 if len(rows)!=4 {t.Fatalf("rows=%d %+v",len(rows),rows)}
 for i,r:=range rows {
  if r.Connector!="RACK-J7"||r.End!="BENCH-P9"||r.EndCavity!=fmt.Sprint(i+1)||r.Color!="WHT"||r.AWG!="24" {t.Fatalf("row %+v",r)}
 }
 if rows[0].Cavity!="A"||rows[1].Cavity!="a" {t.Fatal("cavity case lost")}
 kept,unused:=splitUnusedRows(rows)
 if len(kept)!=4||len(unused)!=0 {t.Fatal("physical spare wires declared unused")}
}

func TestPairedSchematicRejectsInsufficientEvidence(t *testing.T) {
 for _,kind:=range []string{"legacy","one-end","colors-conflict","no-tags","single-row","ambiguous-tag"} {
  t.Run(kind,func(t *testing.T){
   p:=inventedPairedPage()
   // Limit rows to a compact region for these evidence tests.
   for i:=range p.Words {if p.Words[i].Y<770 {p.Words[i].Y=730-float64((i-2)/7)*20}}
   switch kind {
   case "legacy":p.PageAccurate=false
   case "one-end":for i:=range p.Words {if p.Words[i].X==765 {p.Words[i].Text=""}}
   case "colors-conflict":for i:=range p.Words {if p.Words[i].X==765 {p.Words[i].Text="BLK"}}
   case "no-tags":p.Words=p.Words[2:]
   case "single-row":p.Words=p.Words[:9]
   case "ambiguous-tag":p.Words=append(p.Words,Word{Text:"RACK-J8",X:56,Y:770,Size:8})
   }
   if rows:=pairedSchematicRows(p);len(rows)!=0 {t.Fatalf("accepted ambiguous/insufficient evidence: %+v",rows)}
  })
 }
}

func TestPairedSchematicMixedBOMPDF(t *testing.T) {
 // Independent PDF text objects exercise actual PDFium extraction, not just
 // hand-constructed Word records. Both BOM and schematic share one page.
 text:=func(x,y int,s string)string{return fmt.Sprintf("BT /F1 8 Tf 1 0 0 1 %d %d Tm (%s) Tj ET\n",x,y,s)}
 content:=text(55,750,"RACK-J7")+text(790,750,"BENCH-P9")
 for i,pin:=range []string{"A","a","3","4"} {y:=710-i*24
  content+=text(55,y,pin)+text(72,y,"WHT")+text(420,y,"24")+text(440,y,"AWG")+text(765,y,"WHT")+text(790,y,fmt.Sprint(i+1))+text(820,y,"SPARE")
 }
 content+=text(70,330,"ITEM")+text(150,330,"PART NUMBER")+text(350,330,"DESCRIPTION")+text(650,330,"QTY")
 content+=text(70,310,"1")+text(150,310,"EXAMPLE-100")+text(350,310,"TEST MATERIAL")+text(650,310,"2")
 content+=text(70,290,"2")+text(150,290,"EXAMPLE-200")+text(350,290,"TEST LABEL")+text(650,290,"4")
 a,err:=AnalyzeDocument(onePagePDF(content,"[0 0 1000 800]"))
 if err!=nil {t.Fatal(err)}
 if len(a.BOM)!=2||len(a.Wires)!=4 {t.Fatalf("BOM=%+v wires=%+v",a.BOM,a.Wires)}
 for _,r:=range a.Wires {if r.Method!="paired-schematic"||r.AWG!="24"||r.End!="BENCH-P9" {t.Fatalf("row %+v",r)}}
 if len(a.PageSummary)!=1||!strings.Contains(a.PageSummary[0].Category,"BOM + WIRING"){t.Fatalf("summary %+v",a.PageSummary)}
}
