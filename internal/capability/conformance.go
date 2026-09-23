package capability

import "fmt"

type ConformanceStatus string

const (
	ConformancePass ConformanceStatus = "pass"
	ConformanceFail ConformanceStatus = "fail"
)

type ConformanceCheck struct {
	Name    string            `json:"name"`
	Status  ConformanceStatus `json:"status"`
	Message string            `json:"message,omitempty"`
}

type ConformanceReport struct {
	ProviderID      string             `json:"provider_id"`
	ProviderVersion string             `json:"provider_version"`
	Protocol        string             `json:"protocol"`
	Provider        ProviderKind       `json:"provider"`
	Services        []ServiceKind      `json:"services,omitempty"`
	Status          ConformanceStatus  `json:"status"`
	Checks          []ConformanceCheck `json:"checks"`
}

// CheckIntegrationContract performs transport-independent static conformance.
// Runtime capability conformance remains provider/capability specific and will
// build on this report as reference providers are added.
func CheckIntegrationContract(descriptor IntegrationDescriptor) ConformanceReport {
	report := ConformanceReport{
		ProviderID:      descriptor.ID,
		ProviderVersion: descriptor.Version,
		Protocol:        descriptor.Protocol,
		Provider:        descriptor.Provider.Kind,
		Status:          ConformancePass,
	}
	if err := descriptor.Validate(); err != nil {
		report.Status = ConformanceFail
		report.Checks = append(report.Checks, ConformanceCheck{
			Name:    "provider-contract",
			Status:  ConformanceFail,
			Message: err.Error(),
		})
		return report
	}
	services, err := descriptor.EffectiveServices()
	if err != nil {
		report.Status = ConformanceFail
		report.Checks = append(report.Checks, ConformanceCheck{
			Name:    "service-declarations",
			Status:  ConformanceFail,
			Message: err.Error(),
		})
		return report
	}
	report.Services = services
	report.Checks = append(report.Checks,
		ConformanceCheck{Name: "service-declarations", Status: ConformancePass},
		ConformanceCheck{Name: "provider-protocol", Status: ConformancePass},
		ConformanceCheck{Name: "capability-declarations", Status: ConformancePass},
	)
	return report
}

func RequireIntegrationContract(descriptor IntegrationDescriptor) error {
	report := CheckIntegrationContract(descriptor)
	if report.Status == ConformancePass {
		return nil
	}
	if len(report.Checks) == 0 {
		return fmt.Errorf("provider integration contract failed")
	}
	return fmt.Errorf("provider integration contract failed: %s", report.Checks[0].Message)
}
