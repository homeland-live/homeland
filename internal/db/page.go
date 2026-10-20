package db

// Page is a pagination request (Num from 1, Size up to 25) plus the total
// Count the query fills in.
type Page struct {
	Num   int `json:"num" query:"pagenum,default:1" validate:"required,gt=0"`
	Size  int `json:"size" query:"pagesize,default:10" validate:"required,gt=0,lte=25"`
	Count int `json:"count" query:"-"`
}

// Offset returns the number of rows to skip.
func (p *Page) Offset() int {
	return (p.Num - 1) * p.Size
}
