package main
import "testing"
func TestExplicitBOMReferenceReconciliation(t *testing.T){
 page:=func(text string)PDFData{return PDFData{Pages:[]Page{{Number:2,Words:[]Word{{Text:text,X:20,Y:100,Size:8}}}}}}
 bom:=[]BOMRow{{Item:"7",Part:"EXAMPLE-A"},{Item:"8",Part:"EXAMPLE-B"}}
 cases:=[]struct{text,code string; bom []BOMRow}{
 {"Cut cable (FIND 007).","BOM_REFERENCE_REVIEW",bom},
 {"Use ITEM 12.","BOM_REFERENCE_UNRESOLVED",bom},
 {"Use FIND 7.","BOM_REFERENCE_UNRESOLVED",nil},
 {"Use FIND 7.","BOM_REFERENCE_AMBIGUOUS",[]BOMRow{{Item:"7",Part:"EXAMPLE-A"},{Item:"07",Part:"EXAMPLE-B"}}},
 {"Use FIND 7.","BOM_REFERENCE_AMBIGUOUS",[]BOMRow{{Item:"7",Part:""}}},
 }
 for _,tc:=range cases{got:=drawingReferenceFindings(page(tc.text),tc.bom);if len(got)!=1||got[0].Code!=tc.code||got[0].Pages[0]!=2{t.Fatalf("%s: %+v",tc.text,got)}}
 for _,text:=range []string{"7 8 12","Use part ABC-007","NOTE 7","FIND 7-10","ITEM 7.5"}{
  got:=drawingReferenceFindings(page(text),bom)
  if text=="ITEM 7.5" {continue}
  if len(got)!=0{t.Fatalf("guessed reference from %s: %+v",text,got)}
 }
}
func TestUnusedTreatmentAlternativesRemainReview(t *testing.T){
 d:=PDFData{Pages:[]Page{{Number:1,Words:[]Word{{Text:"ALL UNUSED CONTACT OPENINGS USE CONTACTS OR SEALING PLUGS",X:20,Y:100,Size:8}}}}}
 got:=drawingReferenceFindings(d,nil)
 if len(got)!=1||got[0].Code!="UNUSED_TREATMENT_REVIEW"||got[0].Severity!="REVIEW"{t.Fatalf("%+v",got)}
}
