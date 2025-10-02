package prompts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTemplate(t *testing.T) {
	t.Run("empty template", func(t *testing.T) {
		result := ParseTemplate("")
		assert.Equal(t, "", result.OriginalTemplate)
		assert.Empty(t, result.Variables)
	})

	t.Run("no variables", func(t *testing.T) {
		result := ParseTemplate("You are a helpful assistant.")
		assert.Equal(t, "You are a helpful assistant.", result.OriginalTemplate)
		assert.Empty(t, result.Variables)
	})

	t.Run("legacy {{variable}} syntax", func(t *testing.T) {
		result := ParseTemplate("You are a {{role}} assistant.")
		assert.Equal(t, "You are a {{role}} assistant.", result.OriginalTemplate)
		require.Len(t, result.Variables, 1)

		v := result.Variables[0]
		assert.Equal(t, "role", v.Name)
		assert.False(t, v.IsRequired)
		assert.False(t, v.HasDefault)
		assert.Equal(t, "", v.DefaultValue)
		assert.Equal(t, "{{role}}", v.OriginalSyntax)
	})

	t.Run("optional variable with default", func(t *testing.T) {
		result := ParseTemplate("You are a {role:helpful assistant} specializing in {domain:general topics}.")
		assert.Equal(t, "You are a {role:helpful assistant} specializing in {domain:general topics}.", result.OriginalTemplate)
		require.Len(t, result.Variables, 2)

		// Check role variable
		roleVar := result.Variables[0]
		assert.Equal(t, "role", roleVar.Name)
		assert.False(t, roleVar.IsRequired)
		assert.True(t, roleVar.HasDefault)
		assert.Equal(t, "helpful assistant", roleVar.DefaultValue)
		assert.Equal(t, "{role:helpful assistant}", roleVar.OriginalSyntax)

		// Check domain variable
		domainVar := result.Variables[1]
		assert.Equal(t, "domain", domainVar.Name)
		assert.False(t, domainVar.IsRequired)
		assert.True(t, domainVar.HasDefault)
		assert.Equal(t, "general topics", domainVar.DefaultValue)
		assert.Equal(t, "{domain:general topics}", domainVar.OriginalSyntax)
	})

	t.Run("required variable", func(t *testing.T) {
		result := ParseTemplate("You are a {!role} assistant.")
		assert.Equal(t, "You are a {!role} assistant.", result.OriginalTemplate)
		require.Len(t, result.Variables, 1)

		v := result.Variables[0]
		assert.Equal(t, "role", v.Name)
		assert.True(t, v.IsRequired)
		assert.False(t, v.HasDefault)
		assert.Equal(t, "", v.DefaultValue)
		assert.Equal(t, "{!role}", v.OriginalSyntax)
	})

	t.Run("required variable with default", func(t *testing.T) {
		result := ParseTemplate("You are a {!role:-expert} assistant.")
		assert.Equal(t, "You are a {!role:-expert} assistant.", result.OriginalTemplate)
		require.Len(t, result.Variables, 1)

		v := result.Variables[0]
		assert.Equal(t, "role", v.Name)
		assert.True(t, v.IsRequired)
		assert.True(t, v.HasDefault)
		assert.Equal(t, "expert", v.DefaultValue) // Note: should be "expert" not "-expert"
		assert.Equal(t, "{!role:-expert}", v.OriginalSyntax)
	})

	t.Run("mixed syntax", func(t *testing.T) {
		result := ParseTemplate("You are a {{role}} {domain:AI} {!specialization} assistant.")
		assert.Equal(t, "You are a {{role}} {domain:AI} {!specialization} assistant.", result.OriginalTemplate)
		require.Len(t, result.Variables, 3)

		// Check role (legacy)
		roleVar := result.Variables[0]
		assert.Equal(t, "role", roleVar.Name)
		assert.False(t, roleVar.IsRequired)
		assert.False(t, roleVar.HasDefault)
		assert.Equal(t, "{{role}}", roleVar.OriginalSyntax)

		// Check domain (optional with default)
		domainVar := result.Variables[1]
		assert.Equal(t, "domain", domainVar.Name)
		assert.False(t, domainVar.IsRequired)
		assert.True(t, domainVar.HasDefault)
		assert.Equal(t, "AI", domainVar.DefaultValue)
		assert.Equal(t, "{domain:AI}", domainVar.OriginalSyntax)

		// Check specialization (required)
		specVar := result.Variables[2]
		assert.Equal(t, "specialization", specVar.Name)
		assert.True(t, specVar.IsRequired)
		assert.False(t, specVar.HasDefault)
		assert.Equal(t, "{!specialization}", specVar.OriginalSyntax)
	})

	t.Run("duplicate variables", func(t *testing.T) {
		result := ParseTemplate("You are a {role:assistant} {role:helper}.")
		assert.Equal(t, "You are a {role:assistant} {role:helper}.", result.OriginalTemplate)
		require.Len(t, result.Variables, 1) // Should deduplicate

		v := result.Variables[0]
		assert.Equal(t, "role", v.Name)
		assert.False(t, v.IsRequired)
		assert.True(t, v.HasDefault)
		assert.Equal(t, "assistant", v.DefaultValue) // Should use first occurrence
	})
}

func TestTemplateMethods(t *testing.T) {
	template := ParseTemplate("You are a {role:assistant} {!specialization} {domain:AI} helper.")

	t.Run("RequiredVariables", func(t *testing.T) {
		required := template.RequiredVariables()
		require.Len(t, required, 1)
		assert.Equal(t, "specialization", required[0].Name)
	})

	t.Run("OptionalVariables", func(t *testing.T) {
		optional := template.OptionalVariables()
		require.Len(t, optional, 2)

		names := make(map[string]bool)
		for _, v := range optional {
			names[v.Name] = true
		}
		assert.True(t, names["role"])
		assert.True(t, names["domain"])
	})

	t.Run("VariablesWithDefaults", func(t *testing.T) {
		withDefaults := template.VariablesWithDefaults()
		require.Len(t, withDefaults, 2)

		names := make(map[string]bool)
		for _, v := range withDefaults {
			names[v.Name] = true
		}
		assert.True(t, names["role"])
		assert.True(t, names["domain"])
	})

	t.Run("VariablesWithoutDefaults", func(t *testing.T) {
		withoutDefaults := template.VariablesWithoutDefaults()
		require.Len(t, withoutDefaults, 1)
		assert.Equal(t, "specialization", withoutDefaults[0].Name)
	})

	t.Run("VariableNames", func(t *testing.T) {
		names := template.VariableNames()
		require.Len(t, names, 3)

		nameSet := make(map[string]bool)
		for _, name := range names {
			nameSet[name] = true
		}
		assert.True(t, nameSet["role"])
		assert.True(t, nameSet["specialization"])
		assert.True(t, nameSet["domain"])
	})

	t.Run("RequiredVariableNames", func(t *testing.T) {
		names := template.RequiredVariableNames()
		require.Len(t, names, 1)
		assert.Equal(t, "specialization", names[0])
	})

	t.Run("OptionalVariableNames", func(t *testing.T) {
		names := template.OptionalVariableNames()
		require.Len(t, names, 2)

		nameSet := make(map[string]bool)
		for _, name := range names {
			nameSet[name] = true
		}
		assert.True(t, nameSet["role"])
		assert.True(t, nameSet["domain"])
	})
}
