package db

// MaxPageNum is the highest page a query may ask for. At the largest page
// size that is 25 million events, far more than a device keeps, and it keeps
// Offset from overflowing.
const MaxPageNum = 1_000_000

// Page is a pagination request (Num from 1 to MaxPageNum, Size up to 25) plus
// the total Count the query fills in.
type Page struct {
	// Num's lte must match MaxPageNum.
	Num   int `json:"num" query:"pagenum,default:1" validate:"required,gt=0,lte=1000000"`
	Size  int `json:"size" query:"pagesize,default:10" validate:"required,gt=0,lte=25"`
	Count int `json:"count" query:"-"`
}

// Offset returns the number of rows to skip.
func (p *Page) Offset() int {
	return (p.Num - 1) * p.Size
}
