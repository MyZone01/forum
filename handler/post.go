package handler

import (
	"forum/data/models"
	"forum/lib"
	"log"
	"net/http"
	"sort"
	"strings"
	"text/template"
	"time"
)

type PostPageData struct {
	IsLoggedIn     bool
	CurrentUser    models.User
	Post           models.Post
	Comments       []*models.CommentItem
	UserPoster     *models.User
	NbrComment     int
	CategoriesPost []models.Category
	NbrLike        int
	NbrDislike     int
	Categories     []*models.Category
	Allposts       []*models.Post
}

func SortComments(comments []*models.CommentItem) []*models.CommentItem {
	commentMap := make(map[string][]*models.CommentItem)

	for _, comment := range comments {
		commentMap[comment.ParentID] = append(commentMap[comment.ParentID], comment)
	}

	var sortedComments []*models.CommentItem
	var dfs func(string, int)
	dfs = func(parentID string, depth int) {
		children := commentMap[parentID]
		sort.SliceStable(children, func(i, j int) bool {
			return children[i].Index < children[j].Index
		})
		for _, child := range children {
			child.Index = depth
			child.Depth = strings.Repeat(`<span class="ml-1"></span>`, child.Index)
			sortedComments = append(sortedComments, child)
			dfs(child.ID, depth+1)
		}
	}

	dfs("", 0)
	return sortedComments
}

func CreatePost(res http.ResponseWriter, req *http.Request) {
	if lib.ValidateRequest(req, res, "/post", http.MethodPost) {
		isSessionOpen := models.ValidSession(req)
		if isSessionOpen {
			// Parse form data
			err := req.ParseMultipartForm(32 << 20) // 32 MB limit
			if err != nil {
				log.Println("❌ Failed to parse form data", err.Error())
				return
			}
			creationDate := time.Now().Format("2006-01-02 15:04:05")
			modificationDate := time.Now().Format("2006-01-02 15:04:05")
			title := req.FormValue("title")
			posts, err := models.PostRepo.GetAllPosts("")
			if err != nil {
				return
			}
			if posts != nil {
				for j := 0; j < len(posts); j++ {
					if strings.EqualFold(posts[j].Title, title) {
						lib.RedirectToPreviousURL(res, req)
						return
					}
				}
			}
			slug := lib.Slugify(title)
			description := req.FormValue("description")
			_categories := req.FormValue("categories")

			imageUrl := lib.UploadImage(req)
			authorID := models.GetUserFromSession(req).ID
			categories := strings.Split(_categories, "#")

			post := models.Post{
				Title:        title,
				Slug:         slug,
				Description:  description,
				ImageURL:     imageUrl,
				AuthorID:     authorID,
				IsEdited:     false,
				CreateDate:   creationDate,
				ModifiedDate: modificationDate,
			}

			err = models.PostRepo.CreatePost(&post)
			if err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				log.Println("❌ Can't create post")
				return
			}

			for i := 1; i < len(categories); i++ {
				name := strings.TrimSpace(categories[i])
				category, _ := models.CategoryRepo.GetCategoryByName(name)
				if category == nil {
					category = &models.Category{
						Name:         name,
						CreateDate:   creationDate,
						ModifiedDate: modificationDate,
					}
					models.CategoryRepo.CreateCategory(category)
				}
				models.PostCategoryRepo.CreatePostCategory(category.ID, post.ID)
			}

			log.Println("✅ Post created with success")
			lib.RedirectToPreviousURL(res, req)
		}
	}
}

func DeletePost(res http.ResponseWriter, req *http.Request) {
	if lib.ValidateRequest(req, res, "/delete-post/*", http.MethodGet) {
		isSessionOpen := models.ValidSession(req)
		if isSessionOpen {
			path := req.URL.Path
			pathPart := strings.Split(path, "/")
			if len(pathPart) == 3 && pathPart[1] == "delete-post" && pathPart[2] != "" {
				id := pathPart[2]
				models.PostRepo.DeletePost(id)

				log.Println("✅ Post created with success")
				lib.RedirectToPreviousURL(res, req)
			}
		}
	}
}

func GetPost(res http.ResponseWriter, req *http.Request) {
	if lib.ValidateRequest(req, res, "/posts/*", http.MethodGet) {
		basePath := "base"
		pagePath := "post"

		isSessionOpen := models.ValidSession(req)

		err := req.ParseForm()
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		path := req.URL.Path
		pathPart := strings.Split(path, "/")
		if len(pathPart) == 3 && pathPart[1] == "posts" && pathPart[2] != "" {
			slug := pathPart[2]
			post, err := models.PostRepo.GetPostBySlug(slug)
			if post == nil {
				res.WriteHeader(http.StatusNotFound)
				lib.RenderPage("base", "404", nil, res)
				log.Println("404 ❌ - Page not found ", req.URL.Path)
				return
			}
			if err != nil {
				log.Println("❌ error DB", err.Error())
				return
			}
			PostComments, err := models.CommentRepo.GetCommentsOfPost(post.ID, "15")
			if err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				log.Println("❌ error DB", err.Error())
				return
			}
			PostComments = SortComments(PostComments)
			post.ModifiedDate = strings.ReplaceAll(post.ModifiedDate, "T", " ")
			post.ModifiedDate = strings.ReplaceAll(post.ModifiedDate, "Z", "")
			post.ModifiedDate = lib.TimeSinceCreation(post.ModifiedDate)
			userPost, err := models.UserRepo.GetUserByID(post.AuthorID)
			if err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				log.Println("❌ error reading from user", err.Error())
				return
			}
			postCategories, err := models.PostCategoryRepo.GetCategoriesOfPost(post.ID)
			if err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				log.Println("❌ error reading from category", err.Error())
				return
			}
			nbrLike, err := models.ViewRepo.GetLikesByPost(post.ID)
			if err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				log.Println("❌ error reading from View", err.Error())
				return
			}
			nbrDislike, err := models.ViewRepo.GetDislikesByPost(post.ID)
			if err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				log.Println("❌ error reading from View", err.Error())
				return
			}
			post.Description = template.HTMLEscapeString(post.Description)
			post.Title = template.HTMLEscapeString(post.Title)
			if postCategories != nil {
				for k := 0; k < len(postCategories); k++ {
					postCategories[k].Name = template.HTMLEscapeString(postCategories[k].Name)
				}
			}

			if PostComments != nil {
				for j := 0; j < len(PostComments); j++ {
					PostComments[j].Text = template.HTMLEscapeString(PostComments[j].Text)
				}
			}
			userPost.IsLoggedIn = "Offline"
			if models.CheckIfSessionExist(userPost.Username) {
				userPost.IsLoggedIn = "Online"
			}
			cat, err := models.CategoryRepo.GetAllCategory()
			if err != nil {
				return
			}
			allPost, err := models.PostRepo.GetAllPosts("")
			if err != nil {
				return
			}
			PostPageData := PostPageData{
				IsLoggedIn:     isSessionOpen,
				Post:           *post,
				CurrentUser:    *(models.GetUserFromSession(req)),
				UserPoster:     userPost,
				Comments:       PostComments,
				NbrComment:     len(PostComments),
				CategoriesPost: postCategories,
				NbrLike:        nbrLike,
				NbrDislike:     nbrDislike,
				Categories:     cat,
				Allposts:       allPost,
			}
			lib.RenderPage(basePath, pagePath, PostPageData, res)
			log.Println("✅ Post page get with success")
		} else {
			res.WriteHeader(http.StatusNotFound)
			lib.RenderPage("base", "404", nil, res)
			log.Println("404 ❌ - Page not found ", req.URL.Path)
		}
	}
}

func Comment(res http.ResponseWriter, req *http.Request) {
	if lib.ValidateRequest(req, res, "/comment/*", http.MethodPost) {
		err := req.ParseForm()
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		text := strings.TrimSpace(req.FormValue("text"))
		parentID := strings.TrimSpace(req.FormValue("parentID"))
		path := req.URL.Path
		pathPart := strings.Split(path, "/")
		if text != "" {
			if len(pathPart) == 3 && pathPart[1] == "comment" {
				creationDate := time.Now().Format("2006-01-02 15:04:05")
				modifDate := time.Now().Format("2006-01-02 15:04:05")

				authorID := models.GetUserFromSession(req).ID
				postID := pathPart[2]

				commentStruct := models.Comment{
					Text:         text,
					AuthorID:     authorID,
					PostID:       postID,
					ParentID:     parentID,
					CreateDate:   creationDate,
					ModifiedDate: modifDate,
				}

				models.CommentRepo.CreateComment(&commentStruct)
				lib.RedirectToPreviousURL(res, req)
			}
		} else {
			lib.RedirectToPreviousURL(res, req)
		}

	}
}
