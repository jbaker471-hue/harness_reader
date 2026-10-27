package main

import (
 "regexp"
 "sort"
 "strconv"
 "strings"
)

// Paired schematic rows need two cavity/color endpoints, explicit connector
// headings and a repeated column pattern. Every result remains unverified.
var schematicPin = regexp.MustCompile(`^(?:[1-9][0-9]{0,2}|[A-Za-z])$`)
var schematicGauge = regexp.MustCompile(`^(?:[1-2][0-9]|3[0-2])$`)

func schematicColor(s string) string {
 aliases := map[string]string{"BK":"BLK","BLACK":"BLK","BLK":"BLK","WH":"WHT","WHITE":"WHT","WHT":"WHT","RD":"RED","RED":"RED","GN":"GRN","GREEN":"GRN","GRN":"GRN","BU":"BLU","BLUE":"BLU","BLU":"BLU","YE":"YEL","YELLOW":"YEL","YEL":"YEL","OG":"ORN","ORANGE":"ORN","ORN":"ORN","BN":"BRN","BROWN":"BRN","BRN":"BRN","GY":"GRY","GRAY":"GRY","GREY":"GRY","GRY":"GRY","VT":"VIO","VIOLET":"VIO","VIO":"VIO","PK":"PNK","PINK":"PNK","PNK":"PNK"}
 parts:=strings.Split(strings.ToUpper(strings.TrimSpace(s)),"/")
 for i,p:=range parts {v,ok:=aliases[p];if !ok{return ""};parts[i]=v}
 return strings.Join(parts,"/")
}

func schematicWords(line evidenceLine) []Word {
 var out []Word
 for _,w:=range line.Words {
  if w.Rotated {continue}
  fields:=strings.Fields(w.Text)
  cursor:=0
  for _,f:=range fields {
   at:=strings.Index(w.Text[cursor:],f)+cursor
   width:=w.right()-w.X
   out=append(out,Word{Text:f,X:w.X+width*float64(at)/float64(len(w.Text)),X1:w.X+width*float64(at+len(f))/float64(len(w.Text)),Y:w.Y,Size:w.Size})
   cursor=at+len(f)
  }
 }
 sort.SliceStable(out,func(i,j int)bool{return out[i].X<out[j].X})
 return out
}

func schematicTag(p Page,x,y,em float64) string {
 best,score:="",1e12
 ambiguous:=false
 for _,w:=range p.Words {
  text:=strings.ToUpper(strings.TrimSpace(w.Text))
  tag:=reRefDesignator.FindString(text)
  if w.Rotated || tag=="" || tag!=text || w.Y<=y+em || w.Y-y>p.Height*.55 || abs(w.X-x)>p.Width*.12 {continue}
  d:=abs(w.X-x)+(w.Y-y)*.2
  if d<score-em {best,score,ambiguous=tag,d,false
  }else if abs(d-score)<=em && tag!=best {ambiguous=true}
 }
 if ambiguous{return ""}
 return best
}

func pairedSchematicRows(p Page) []WireRow {
 if !p.PageAccurate || p.Width<=0 || p.Height<=0{return nil}
 type endpoint struct {pin,color Word; index int}
 type row struct {wire WireRow; x1,x2,em float64}
 var candidates []row
 for _,line:=range pageLines(p) {
  words:=schematicWords(line)
  em:=lineEm(line)
  var left,right []endpoint
  for i:=0;i+1<len(words);i++ {
   a,b:=words[i],words[i+1]
   if b.X-a.right()>4*em || b.X<=a.X {continue}
   if schematicPin.MatchString(a.Text)&&schematicColor(b.Text)!="" {left=append(left,endpoint{a,b,i})}
   if schematicColor(a.Text)!=""&&schematicPin.MatchString(b.Text) {right=append(right,endpoint{b,a,i+1})}
  }
  // Multiple possible endpoint pairs are ambiguous, not a reason to choose
  // the first number or silently discard the other possible endpoints.
  if len(left)!=1||len(right)!=1 {continue}
  a,b:=left[0],right[0]
  if b.pin.X-a.pin.X<p.Width*.15 || a.color.X>=b.color.X {continue}
  color:=schematicColor(a.color.Text)
  if color!=schematicColor(b.color.Text){continue}
  from,to:=schematicTag(p,a.pin.X,line.Y,em),schematicTag(p,b.pin.X,line.Y,em)
  if from==""||to==""||from==to {continue}
  gauge:=""
  gauges:=map[string]bool{}
  for i,w:=range words {
   if w.X<=a.color.right()+em || w.right()>=b.color.X-em {continue}
   if schematicGauge.MatchString(w.Text) && ((i>0&&strings.EqualFold(words[i-1].Text,"AWG"))||(i+1<len(words)&&strings.EqualFold(words[i+1].Text,"AWG"))) {gauges[w.Text]=true}
  }
  if len(gauges)==1 {for g:=range gauges {gauge=g}}
  before,after:=[]string{},[]string{}
  for i,w:=range words {if i<a.index {before=append(before,w.Text)};if i>b.index {after=append(after,w.Text)}}
  candidates=append(candidates,row{WireRow{Page:p.Number,Connector:from,Cavity:a.pin.Text,Signal:strings.Join(before," "),End:to,EndCavity:b.pin.Text,EndSignal:strings.Join(after," "),Color:color,AWG:gauge,Y:line.Y,Method:"paired-schematic",Confidence:"DRAFT — PAIRED SCHEMATIC ENDPOINTS; CHECK SOURCE"},a.pin.X,b.pin.X,em})
 }
 var out []WireRow
 for _,r:=range candidates {
  peers:=map[string]bool{}
  for _,v:=range candidates {
   if v.wire.Connector==r.wire.Connector&&v.wire.End==r.wire.End&&abs(v.x1-r.x1)<=2*r.em&&abs(v.x2-r.x2)<=2*r.em {
    peers[strconv.FormatFloat(v.wire.Y,'f',1,64)]=true
   }
  }
  if len(peers)>=3 {out=append(out,r.wire)}
 }
 return out
}
