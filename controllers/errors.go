package controllers

import "errors"

var (
	TargetGroupNameErr = errors.New("wrong group name")
	TargetKindErr      = errors.New("target kind error")
	TargetRefNotExists = errors.New("targetRef does not exists")
	TargetRefConflict  = errors.New("targetRef has conflict")
)
