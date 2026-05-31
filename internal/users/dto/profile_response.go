package dto

type ProfileResponse struct {
	Username    string `json:"username"`
	Role        string `json:"role"`
	ClassName   string `json:"className,omitempty"`   // student only
	TeacherName string `json:"teacherName,omitempty"` // teacher only
	TeacherID   *uint  `json:"teacherId,omitempty"`   // teacher only
}
