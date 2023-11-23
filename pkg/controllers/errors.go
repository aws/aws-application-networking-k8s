package controllers

import "errors"

var (
	GroupNameError    = errors.New("wrong group name")
	KindError         = errors.New("target kind error")
	TargetRefNotFound = errors.New("targetRef not found")
	TargetRefConflict = errors.New("targetRef has conflict")
)
