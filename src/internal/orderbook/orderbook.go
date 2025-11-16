package orderbook




type Order struct {
  idNumber int
  buyOrSell int
  shares int
  limit int
  entryTime int
  eventTime int
  nextOrder *int
  prevOrder *int
  parentLimit *int
}

type Limit struct {
  limitPrice int
  size int
  totalVolume int
  parent *int
  leftChild *int
  rightChild *int
  headOrder *int
  tailOrder *int
}

type Book struct {
  buyTree *int
  sellTree *int
  lowestSell *int
  highestBuy *int
}







func (* Book) add(buy bool, shares, limit, entryTime, eventTime int) {
	
}

