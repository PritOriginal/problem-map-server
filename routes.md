# github.com/PritOriginal/problem-map-server

REST generated docs.

## Routes

<details>
<summary>`/auth/signin`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/auth**
	- **/signin**
		- _POST_
			- [github.com/PritOriginal/problem-map-server/internal/handler/auth.Register.func1.(*handler).SignIn.2]()

</details>
<details>
<summary>`/auth/signup`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/auth**
	- **/signup**
		- _POST_
			- [github.com/PritOriginal/problem-map-server/internal/handler/auth.Register.func1.(*handler).SignUp.1]()

</details>
<details>
<summary>`/auth/tokens/refresh`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/auth**
	- **/tokens/refresh**
		- _POST_
			- [github.com/PritOriginal/problem-map-server/internal/handler/auth.Register.func1.(*handler).RefreshTokens.3]()

</details>
<details>
<summary>`/map/cities`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/map**
	- **/cities**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.(*handler).GetCities.3]()

</details>
<details>
<summary>`/map/districts`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/map**
	- **/districts**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.(*handler).GetDistricts.4]()

</details>
<details>
<summary>`/map/marks`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/map**
	- **/marks**
		- **/**
			- _POST_
				- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.1.1.Verifier.Verify.4]()
				- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.1.1.Authenticator.1]()
				- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.1.1.(*handler).AddMark.2]()
			- _GET_
				- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.1.(*handler).GetMarks.2]()

</details>
<details>
<summary>`/map/marks/photos`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/map**
	- **/marks**
		- **/photos**
			- _POST_
				- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.1.1.Verifier.Verify.4]()
				- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.1.1.Authenticator.1]()
				- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.1.1.(*handler).AddPhotos.3]()

</details>
<details>
<summary>`/map/regions`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/map**
	- **/regions**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/map.Register.func1.(*handler).GetRegions.2]()

</details>
<details>
<summary>`/tasks`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/tasks**
	- **/**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/tasks.Register.func1.(*handler).GetTasks.1]()
		- _POST_
			- [github.com/PritOriginal/problem-map-server/internal/handler/tasks.Register.func1.(*handler).AddTask.4]()

</details>
<details>
<summary>`/tasks/user/{id}`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/tasks**
	- **/user/{id}**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/tasks.Register.func1.(*handler).GetTasksByUserId.3]()

</details>
<details>
<summary>`/tasks/{id}`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/tasks**
	- **/{id}**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/tasks.Register.func1.(*handler).GetTaskById.2]()

</details>
<details>
<summary>`/users`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/users**
	- **/**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/users.Register.func1.(*handler).GetUsers.1]()

</details>
<details>
<summary>`/users/{id}`</summary>

- [RequestID]()
- [RealIP]()
- [github.com/PritOriginal/problem-map-server/internal/handler.GetRouter.New.func1]()
- [Recoverer]()
- [URLFormat]()
- **/users**
	- **/{id}**
		- _GET_
			- [github.com/PritOriginal/problem-map-server/internal/handler/users.Register.func1.(*handler).GetUserById.2]()

</details>

Total # of routes: 13
