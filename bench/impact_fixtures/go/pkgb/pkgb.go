package pkgb

func Collide() string {
	return "b"
}

func UseB() string {
	return Collide()
}
