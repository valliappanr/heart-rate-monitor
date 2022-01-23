package main

type MeterDataInput struct {
	Timestamp  CustomTime `json:"timestamp"`
	Value      float32   `json:"value"`
}

type MeterDataResponse struct {
	Timestamp  CustomTime `json:"timestamp"`
	Value      float32   `json:"value"`
}

type MeterDataRequest struct {
	Timestamp CustomTime `json:"timestamp"`
}
