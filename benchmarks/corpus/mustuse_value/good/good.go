package good

//effectgo:mustuse
type Receipt struct {
	ID int
}

type Issuer struct {
	NextID int
}

func (issuer *Issuer) Issue() Receipt {
	issuer.NextID++
	return Receipt{ID: issuer.NextID}
}

func Purchase(issuer *Issuer) Receipt {
	return issuer.Issue()
}
